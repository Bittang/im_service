/**
 * Copyright (c) 2014-2015, GoBelieve     
 * All rights reserved.
 *
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation; either version 2 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software
 * Foundation, Inc., 59 Temple Place, Suite 330, Boston, MA  02111-1307  USA
 */

package main

import "net"
import "time"
import "sync"
import "sync/atomic"
import log "github.com/golang/glog"
import "github.com/googollee/go-engine.io"

const CLIENT_TIMEOUT = (60 * 6)



type Client struct {
	//客户端协议版本号
	version int
	tm     time.Time
	wt     chan *Message
	ewt    chan *EMessage //在线消息
	owt    chan *EMessage //离线消息

	room_id int64

	appid  int64
	uid    int64
	device_id string
	device_ID int64 //generated by device_id + platform_id
	platform_id int8
	conn   interface{}

	unackMessages map[int]*EMessage
	unacks map[int]int64
	mutex  sync.Mutex
}

func NewClient(conn interface{}) *Client {
	client := new(Client)
	client.conn = conn // conn is net.Conn or engineio.Conn
	client.wt = make(chan *Message, 10)
	client.ewt = make(chan *EMessage, 10)
	client.owt = make(chan *EMessage, 10)

	client.unacks = make(map[int]int64)
	client.unackMessages = make(map[int]*EMessage)
	atomic.AddInt64(&server_summary.nconnections, 1)
	return client
}

func (client *Client) Read() {
	for {
		msg := client.read()
		if msg == nil {
			client.HandleRemoveClient()
			break
		}
		client.HandleMessage(msg)
	}
}

func (client *Client) HandleRemoveClient() {
	client.wt <- nil
	route := app_route.FindRoute(client.appid)
	if route == nil {
		log.Warning("can't find app route")
		return
	}
	route.RemoveClient(client)
	if client.room_id > 0 {
		route.RemoveRoomClient(client.room_id, client)
		channel := GetRoomChannel(client.room_id)
		channel.UnsubscribeRoom(client.appid, client.room_id)
	}
	if client.uid > 0 {
		channel := GetChannel(client.uid)
		channel.Unsubscribe(client.appid, client.uid)
		channel = GetUserStorageChannel(client.uid)
		channel.Unsubscribe(client.appid, client.uid)
		client.UnsubscribeGroup()
	}
}

func (client *Client) HandleMessage(msg *Message) {
	log.Info("msg cmd:", Command(msg.cmd))
	switch msg.cmd {
	case MSG_AUTH_TOKEN:
		client.HandleAuthToken(msg.body.(*AuthenticationToken), msg.version)
	case MSG_IM:
		client.HandleIMMessage(msg.body.(*IMMessage), msg.seq)
	case MSG_GROUP_IM:
		client.HandleGroupIMMessage(msg.body.(*IMMessage), msg.seq)
	case MSG_ACK:
		client.HandleACK(msg.body.(*MessageACK))
	case MSG_HEARTBEAT:
		// nothing to do
	case MSG_PING:
		client.HandlePing()
	case MSG_INPUTING:
		client.HandleInputing(msg.body.(*MessageInputing))
	case MSG_SUBSCRIBE_ONLINE_STATE:
		client.HandleSubsribe(msg.body.(*MessageSubscribeState))
	case MSG_RT:
		client.HandleRTMessage(msg)
	case MSG_ENTER_ROOM:
		client.HandleEnterRoom(msg.body.(*Room))
	case MSG_LEAVE_ROOM:
		client.HandleLeaveRoom(msg.body.(*Room))
	case MSG_ROOM_IM:
		client.HandleRoomIM(msg.body.(*RoomMessage), msg.seq)
	default:
		log.Info("unknown msg:", msg.cmd)
	}
}

func (client *Client) LoadGroupOfflineMessage(gid int64) ([]*EMessage, error) {
	storage_pool := GetGroupStorageConnPool(gid)
	storage, err := storage_pool.Get()
	if err != nil {
		log.Error("connect storage err:", err)
		return nil, err
	}
	defer storage_pool.Release(storage)

	return storage.LoadGroupOfflineMessage(client.appid, gid, client.uid, client.device_ID)
}

func (client *Client) LoadGroupOffline() {
	if client.device_ID == 0 {
		return
	}

	groups := group_manager.FindUserGroups(client.appid, client.uid)
	for _, group := range groups {
		if !group.super {
			continue
		}
		messages, err := client.LoadGroupOfflineMessage(group.gid)
		if err != nil {
			log.Errorf("load group offline message err:%d %s", group.gid, err)
			continue
		}

		for _, emsg := range messages {
			client.owt <- emsg
		}
	}
}

func (client *Client) LoadOffline() {
	if client.device_ID == 0 {
		return
	}
	storage_pool := GetStorageConnPool(client.uid)
	storage, err := storage_pool.Get()
	if err != nil {
		log.Error("connect storage err:", err)
		return
	}
	defer storage_pool.Release(storage)

	messages, err := storage.LoadOfflineMessage(client.appid, client.uid, client.device_ID)
	if err != nil {
		log.Errorf("load offline message err:%d %s", client.uid, err)
		return
	}
	for _, emsg := range messages {
		client.owt <- emsg
	}
}

func (client *Client) SubscribeGroup() {
	group_center.SubscribeGroup(client.appid, client.uid)
}

func (client *Client) UnsubscribeGroup() {
	group_center.UnsubscribeGroup(client.appid, client.uid)
}

func (client *Client) SendMessage(uid int64, msg *Message) bool {
	return Send0Message(client.appid, uid, msg)
}

func (client *Client) SendLoginPoint() {
	point := &LoginPoint{}
	point.up_timestamp = int32(client.tm.Unix())
	point.platform_id = client.platform_id
	point.device_id = client.device_id
	msg := &Message{cmd:MSG_LOGIN_POINT, body:point}
	client.SendMessage(client.uid, msg)
}

func (client *Client) AuthToken(token string) (int64, int64, error) {
	appid, uid, _, err := LoadUserAccessToken(token)
	return appid, uid, err
}


func (client *Client) HandleAuthToken(login *AuthenticationToken, version int) {
	if client.uid > 0 {
		log.Info("repeat login")
		return
	}

	var err error
	client.appid, client.uid, err = client.AuthToken(login.token)
	if err != nil {
		log.Info("auth token err:", err)
		msg := &Message{cmd: MSG_AUTH_STATUS, body: &AuthenticationStatus{1}}
		client.wt <- msg
		return
	}
	if  client.uid == 0 {
		log.Info("auth token uid==0")
		msg := &Message{cmd: MSG_AUTH_STATUS, body: &AuthenticationStatus{1}}
		client.wt <- msg
		return
	}

	if login.platform_id != PLATFORM_WEB && len(login.device_id) > 0{
		client.device_ID, err = GetDeviceID(login.device_id, int(login.platform_id))
		if err != nil {
			log.Info("auth token uid==0")
			msg := &Message{cmd: MSG_AUTH_STATUS, body: &AuthenticationStatus{1}}
			client.wt <- msg
			return
		}
	}

	client.version = version
	client.device_id = login.device_id
	client.platform_id = login.platform_id
	client.tm = time.Now()
	log.Infof("auth token:%s appid:%d uid:%d device id:%s:%d", 
		login.token, client.appid, client.uid, client.device_id, client.device_ID)

	msg := &Message{cmd: MSG_AUTH_STATUS, body: &AuthenticationStatus{0}}
	client.wt <- msg

	client.SendLoginPoint()
	client.AddClient()

	client.SubscribeGroup()
	channel := GetUserStorageChannel(client.uid)
	channel.Subscribe(client.appid, client.uid)
	channel = GetChannel(client.uid)
	channel.Subscribe(client.appid, client.uid)

	client.LoadOffline()
	client.LoadGroupOffline()
	
	close(client.owt)
	log.Infof("offline loaded:%d", client.uid)

	CountDAU(client.appid, client.uid)
	atomic.AddInt64(&server_summary.nclients, 1)
}

func (client *Client) AddClient() {
	route := app_route.FindOrAddRoute(client.appid)
	route.AddClient(client)
}

func (client *Client) HandleSubsribe(msg *MessageSubscribeState) {
	if client.uid == 0 {
		return
	}

	//todo 获取在线状态
	for _, uid := range msg.uids {
		state := &MessageOnlineState{uid, 0}
		m := &Message{cmd: MSG_ONLINE_STATE, body: state}
		client.wt <- m
	}
}

func (client *Client) HandleRTMessage(msg *Message) {
	rt := msg.body.(*RTMessage)
	if rt.sender != client.uid {
		log.Warningf("rt message sender:%d client uid:%d\n", rt.sender, client.uid)
		return
	}
	
	m := &Message{cmd:MSG_RT, body:rt}
	client.SendMessage(rt.receiver, m)

	atomic.AddInt64(&server_summary.in_message_count, 1)
	log.Infof("realtime message sender:%d receiver:%d", rt.sender, rt.receiver)
}

func (client *Client) HandleIMMessage(msg *IMMessage, seq int) {
	if client.uid == 0 {
		log.Warning("client has't been authenticated")
		return
	}

	if msg.sender != client.uid {
		log.Warningf("im message sender:%d client uid:%d\n", msg.sender, client.uid)
		return
	}
	msg.timestamp = int32(time.Now().Unix())
	m := &Message{cmd: MSG_IM, version:DEFAULT_VERSION, body: msg}

	msgid, err := SaveMessage(client.appid, msg.receiver, client.device_ID, m)
	if err != nil {
		return
	}

	//保存到自己的消息队列，这样用户的其它登陆点也能接受到自己发出的消息
	SaveMessage(client.appid, msg.sender, client.device_ID, m)

	client.wt <- &Message{cmd: MSG_ACK, body: &MessageACK{int32(seq)}}

	atomic.AddInt64(&server_summary.in_message_count, 1)
	log.Infof("peer message sender:%d receiver:%d msgid:%d\n", msg.sender, msg.receiver, msgid)
}

func (client *Client) HandleGroupIMMessage(msg *IMMessage, seq int) {
	if client.uid == 0 {
		log.Warning("client has't been authenticated")
		return
	}

	msg.timestamp = int32(time.Now().Unix())
	m := &Message{cmd: MSG_GROUP_IM, version:DEFAULT_VERSION, body: msg}

	group := group_manager.FindGroup(msg.receiver)
	if group == nil {
		log.Warning("can't find group:", msg.receiver)
		return
	}
	if group.super {
		_, err := SaveGroupMessage(client.appid, msg.receiver, client.device_ID, m)
		if err != nil {
			return
		}
	} else {
		members := group.Members()
		for member := range members {
			_, err := SaveMessage(client.appid, member, client.device_ID, m)
			if err != nil {
				continue
			}
		}
	}
	
	client.wt <- &Message{cmd: MSG_ACK, body: &MessageACK{int32(seq)}}
	atomic.AddInt64(&server_summary.in_message_count, 1)
	log.Infof("group message sender:%d group id:%d", msg.sender, msg.receiver)
}

func (client *Client) HandleInputing(inputing *MessageInputing) {
	msg := &Message{cmd: MSG_INPUTING, body: inputing}
	client.SendMessage(inputing.receiver, msg)
	log.Infof("inputting sender:%d receiver:%d", inputing.sender, inputing.receiver)
}

func (client *Client) HandleACK(ack *MessageACK) {
	log.Info("ack:", ack)
	emsg := client.RemoveUnAckMessage(ack)
	if emsg == nil || emsg.msgid == 0 {
		return
	}

	msg := emsg.msg
	if msg != nil && msg.cmd == MSG_GROUP_IM {
		im := emsg.msg.body.(*IMMessage)
		group := group_manager.FindGroup(im.receiver)
		if group != nil && group.super {
			client.DequeueGroupMessage(emsg.msgid, im.receiver)
		} else {
			client.DequeueMessage(emsg.msgid)
		}
	} else {
		client.DequeueMessage(emsg.msgid)
	}

	if msg == nil {
		return
	}
}

func (client *Client) HandleEnterRoom(room *Room){
	if client.uid == 0 {
		log.Warning("client has't been authenticated")
		return
	}

	room_id := room.RoomID()
	log.Info("enter room id:", room_id)
	if room_id == 0 || client.room_id == room_id {
		return
	}
	route := app_route.FindOrAddRoute(client.appid)
	if client.room_id > 0 {
		channel := GetRoomChannel(client.room_id)
		channel.UnsubscribeRoom(client.appid, client.room_id)
		route.RemoveRoomClient(client.room_id, client)
	}

	client.room_id = room_id
	route.AddRoomClient(client.room_id, client)
	channel := GetRoomChannel(client.room_id)
	channel.SubscribeRoom(client.appid, client.room_id)
}

func (client *Client) HandleLeaveRoom(room *Room) {
	if client.uid == 0 {
		log.Warning("client has't been authenticated")
		return
	}

	room_id := room.RoomID()
	log.Info("leave room id:", room_id)
	if room_id == 0 {
		return
	}
	if client.room_id != room_id {
		return
	}

	route := app_route.FindOrAddRoute(client.appid)
	route.RemoveRoomClient(client.room_id, client)
	channel := GetRoomChannel(client.room_id)
	channel.UnsubscribeRoom(client.appid, client.room_id)
	client.room_id = 0
}

func (client *Client) HandleRoomIM(room_im *RoomMessage, seq int) {
	if client.uid == 0 {
		log.Warning("client has't been authenticated")
		return
	}
	room_id := room_im.receiver
	if room_id != client.room_id {
		log.Warningf("room id:%d is't client's room id:%d\n", room_id, client.room_id)
		return
	}

	m := &Message{cmd:MSG_ROOM_IM, body:room_im}
	route := app_route.FindOrAddRoute(client.appid)
	clients := route.FindRoomClientSet(room_id)
	for c, _ := range(clients) {
		if c == client {
			continue
		}
		c.wt <- m
	}

	amsg := &AppMessage{appid:client.appid, receiver:room_id, msg:m}
	channel := GetRoomChannel(client.room_id)
	channel.PublishRoom(amsg)

	client.wt <- &Message{cmd: MSG_ACK, body: &MessageACK{int32(seq)}}
}

func (client *Client) HandlePing() {
	m := &Message{cmd: MSG_PONG}
	client.wt <- m
	if client.uid == 0 {
		log.Warning("client has't been authenticated")
		return
	}
}

func (client *Client) DequeueGroupMessage(msgid int64, gid int64) {
	storage_pool := GetGroupStorageConnPool(gid)
	storage, err := storage_pool.Get()
	if err != nil {
		log.Error("connect storage err:", err)
		return
	}
	defer storage_pool.Release(storage)

	dq := &DQGroupMessage{msgid:msgid, appid:client.appid, receiver:client.uid, gid:gid, device_id:client.device_ID}
	err = storage.DequeueGroupMessage(dq)
	if err != nil {
		log.Error("dequeue message err:", err)
	}
}

func (client *Client) DequeueMessage(msgid int64) {
	if client.device_ID == 0 {
		return
	}
	
	storage_pool := GetStorageConnPool(client.uid)
	storage, err := storage_pool.Get()
	if err != nil {
		log.Error("connect storage err:", err)
		return
	}
	defer storage_pool.Release(storage)

	dq := &DQMessage{msgid:msgid, appid:client.appid, receiver:client.uid, device_id:client.device_ID}
	err = storage.DequeueMessage(dq)
	if err != nil {
		log.Error("dequeue message err:", err)
	}
}

func (client *Client) RemoveUnAckMessage(ack *MessageACK) *EMessage {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	var msgid int64
	var msg *Message
	var ok bool

	seq := int(ack.seq)
	if msgid, ok = client.unacks[seq]; ok {
		log.Infof("dequeue offline msgid:%d uid:%d\n", msgid, client.uid)
		delete(client.unacks, seq)
	} else {
		log.Warning("can't find msgid with seq:", seq)
	}
	if emsg, ok := client.unackMessages[seq]; ok {
		msg = emsg.msg
		delete(client.unackMessages, seq)
	}

	return &EMessage{msgid:msgid, msg:msg}
}

func (client *Client) AddUnAckMessage(emsg *EMessage) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	seq := emsg.msg.seq
	client.unacks[seq] = emsg.msgid
	if emsg.msg.cmd == MSG_IM || emsg.msg.cmd == MSG_GROUP_IM {
		client.unackMessages[seq] = emsg
	}
}

func (client *Client) Write() {
	seq := 0
	running := true
	loaded := false

	//发送离线消息
	for running && !loaded {
		select {
		case msg := <-client.wt:
			if msg == nil {
				client.close()
				atomic.AddInt64(&server_summary.nconnections, -1)
				if client.uid > 0 {
					atomic.AddInt64(&server_summary.nclients, -1)
				}
				running = false
				log.Infof("client:%d socket closed", client.uid)
				break
			}
			if msg.cmd == MSG_RT {
				atomic.AddInt64(&server_summary.out_message_count, 1)
			}
			seq++

			//以当前客户端所用版本号发送消息
			vmsg := &Message{msg.cmd, seq, client.version, msg.body}
			client.send(vmsg)
		case emsg, ok := <- client.owt:
			if !ok {
				//离线消息读取完毕
				loaded = true
				break
			}
			seq++

			emsg.msg.seq = seq
			client.AddUnAckMessage(emsg)

			//以当前客户端所用版本号发送消息
			msg := &Message{emsg.msg.cmd, seq, client.version, emsg.msg.body}
			if msg.cmd == MSG_IM || msg.cmd == MSG_GROUP_IM {
				atomic.AddInt64(&server_summary.out_message_count, 1)
			}
			client.send(msg)
		}
	}
	
	//发送在线消息
	for running {
		select {
		case msg := <-client.wt:
			if msg == nil {
				client.close()
				atomic.AddInt64(&server_summary.nconnections, -1)
				if client.uid > 0 {
					atomic.AddInt64(&server_summary.nclients, -1)
				}
				running = false
				log.Infof("client:%d socket closed", client.uid)
				break
			}
			if msg.cmd == MSG_RT {
				atomic.AddInt64(&server_summary.out_message_count, 1)
			}
			seq++

			//以当前客户端所用版本号发送消息
			vmsg := &Message{msg.cmd, seq, client.version, msg.body}
			client.send(vmsg)
		case emsg := <- client.ewt:
			seq++

			emsg.msg.seq = seq
			client.AddUnAckMessage(emsg)

			//以当前客户端所用版本号发送消息
			msg := &Message{cmd:emsg.msg.cmd, seq:seq, version:client.version, body:emsg.msg.body}
			if msg.cmd == MSG_IM || msg.cmd == MSG_GROUP_IM {
				atomic.AddInt64(&server_summary.out_message_count, 1)
			}
			client.send(msg)
		}
	}
}

// 根据连接类型获取消息
func (client *Client) read() *Message {
	if conn, ok := client.conn.(net.Conn); ok {
		conn.SetDeadline(time.Now().Add(CLIENT_TIMEOUT * time.Second))
		return ReceiveMessage(conn)
	} else if conn, ok := client.conn.(engineio.Conn); ok {
		return ReadEngineIOMessage(conn)
	}
	return nil
}

// 根据连接类型发送消息
func (client *Client) send(msg *Message) {
	if conn, ok := client.conn.(net.Conn); ok {
		err := SendMessage(conn, msg)
		if err != nil {
			log.Info("send msg:", Command(msg.cmd),  " tcp err:", err)
		}
	} else if conn, ok := client.conn.(engineio.Conn); ok {
		SendEngineIOBinaryMessage(conn, msg)
	}
}

// 根据连接类型关闭
func (client *Client) close() {
	if conn, ok := client.conn.(net.Conn); ok {
		conn.Close()
	} else if conn, ok := client.conn.(engineio.Conn); ok {
		conn.Close()
	}
}

func (client *Client) Run() {
	go client.Write()
	go client.Read()
}
