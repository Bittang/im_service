
#im service
1. 支持点对点消息, 群组消息, 聊天室消息
2. 支持集群部署
3. 单机支持50w用户在线
4. 单机处理消息5000条/s
5. 支持超大群组(3000人)


##编译运行

1. 安装go编译环境
参考链接:https://golang.org/doc/install


2. 下载im_service代码
  cd $GOPATH/src/github.com/GoBelieveIO
  git clone https://github.com/GoBelieveIO/im_service.git

  
3  编译proto文件
   cd im_service 


##编译推送的proto文件
1. go
protoc -IYuanXin_PushService_Greeter/ YuanXin_PushService_Greeter/PushService.proto --go_out=plugins=grpc:YuanXin_PushService_Greeter

python -m grpc.tools.protoc -IYuanXin_PushService_Greeter/ --python_out=YuanXin_PushService_Greeter/ --grpc_python_out=YuanXin_PushService_Greeter/ YuanXin_PushService_Greeter/PushService.proto
