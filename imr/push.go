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
import "fmt"
import "encoding/json"
import log "github.com/golang/glog"
import pb "github.com/GoBelieveIO/im_service/YuanXin_PushService_Greeter"
import "golang.org/x/net/context"
import "google.golang.org/grpc"


func (client *Client) IsROMApp(appid int64) bool {
	return false
}


//离线消息入apns队列
func (client *Client) PublishPeerMessage(appid int64, im *IMMessage) {
	p := &pb.PushModel{}
	p.Appid = fmt.Sprintf("%d", appid)
	p.Title = ""
	p.Alert = im.content
	p.Type = 4
	p.Userids = fmt.Sprintf("%d", im.receiver)
	p.Sender = im.sender

	conn, err := grpc.Dial(config.push_rpc_address, grpc.WithInsecure())
	if err != nil {
		log.Warning("dial push rpc service err:", err)
		return
	}
	defer conn.Close()

	c := pb.NewServiceClient(conn)

	r, err := c.GrpcPushMessage(context.Background(), p)
	if err != nil {
		log.Error("push error:", err)
		return
	}

	log.Infof("push status:%d", r.Stauts)
}

func (client *Client) PublishGroupMessage(appid int64, receivers []int64, im *IMMessage) {
	p := &pb.PushModel{}
	p.Appid = fmt.Sprintf("%d", appid)
	p.Title = ""
	p.Alert = im.content
	p.Type = 4
	userids := ""
	for _, m := range receivers {
		if userids != "" {
			userids += fmt.Sprintf(",%d", m)
		} else {
			userids += fmt.Sprintf("%d", m)
		}
	}
	p.Userids = userids
	p.Sender = im.sender
	p.Groupid = im.receiver

	conn, err := grpc.Dial(config.push_rpc_address, grpc.WithInsecure())
	if err != nil {
		log.Warning("dial push rpc service err:", err)
		return
	}
	defer conn.Close()

	c := pb.NewServiceClient(conn)

	r, err := c.GrpcPushMessage(context.Background(), p)
	if err != nil {
		log.Error("push error:", err)
		return
	}

	log.Infof("push status:%d", r.Stauts)
}

func (client *Client) PublishCustomerMessage(appid, receiver int64, cs *CustomerMessage, cmd int) {
	conn := redis_pool.Get()
	defer conn.Close()

	v := make(map[string]interface{})
	v["appid"] = appid
	v["receiver"] = receiver
	v["command"] = cmd
	v["customer_appid"] = cs.customer_appid
	v["customer"] = cs.customer_id
	v["seller"] = cs.seller_id
	v["store"] = cs.store_id
	v["content"] = cs.content

	b, _ := json.Marshal(v)
	var queue_name string
	queue_name = "customer_push_queue"
	_, err := conn.Do("RPUSH", queue_name, b)
	if err != nil {
		log.Info("rpush error:", err)
	}
}


func (client *Client) PublishSystemMessage(appid, receiver int64, content string) {
	p := &pb.PushSystemModel{}
	p.Appid = fmt.Sprintf("%d",appid)
	p.Uid = receiver
	p.Content = content

	conn, err := grpc.Dial(config.push_rpc_address, grpc.WithInsecure())
	if err != nil {
		log.Warning("dial push rpc service err:", err)
		return
	}
	defer conn.Close()

	c := pb.NewServiceClient(conn)

	r, err := c.GrpcPushSystemMessage(context.Background(), p)
	if err != nil {
		log.Error("push error:", err)
		return
	}

	log.Infof("push status:%d", r.Stauts)
}
