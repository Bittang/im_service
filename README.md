
#im service
1. 支持点对点消息, 群组消息, 聊天室消息
2. 支持集群部署
3. 单机支持50w用户在线
4. 单机处理消息5000条/s
5. 支持超大群组(3000人)


##编译运行

1. 安装go以及依赖包
2. make
3. 安装mysql数据库, redis, 并导入db.sql
4. 修改配置文件
5. ./im im.cfg;./ims ims.cfg; ./imr imr.cfg

注:工程中未包含平台的数据库结构文件


##编译proto文件
1. go
protoc -Irpc/ rpc/rpc.proto --go_out=plugins=grpc:rpc

2. python
python -m grpc.tools.protoc -Irpc --python_out=rpc/ --grpc_python_out=rpc/ rpc/rpc.proto


##编译推送的proto文件
1. go
protoc -IYuanXin_PushService_Greeter/ YuanXin_PushService_Greeter/PushService.proto --go_out=plugins=grpc:YuanXin_PushService_Greeter

python -m grpc.tools.protoc -IYuanXin_PushService_Greeter/ --python_out=YuanXin_PushService_Greeter/ --grpc_python_out=YuanXin_PushService_Greeter/ YuanXin_PushService_Greeter/PushService.proto
