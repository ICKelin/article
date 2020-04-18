# 基于grpc的推送

## grpc

![](https://grpc.io/img/landing-2.svg)

grpc是我在工作当中广泛用到的一个rpc框架，由于公司内部采用微服务架构，服务之间交互使用的就是grpc框架，grpc是一个框架，整合了http2，protobuf等技术，整出了一套对开发人员相对比较友好的框架。

grpc本身使用特别简单，简单到什么程度呢？简单到我刚到公司的时候只听过grpc是什么，摸索一天左右就能开始用grpc写一些demo，第二天就可以进行小功能开发。

grpc本身是基于http2的，相比较http1而言，http2有几个比较重要的的特性:

**1. http2采用二进制传输**

通常在写网络程序时，都会有一个编解码相关的代码，http1的编解码没有太多规范，可能是因为诞生得比较早，http1传输不加密，传输内容也是纯文本，通过换行符来做分割，http2对此有所改进，不在采用纯文本的方式进行传输，而采用二进制方式，我觉得可以类比json和protobuf两种编解码方式。http1类似json，比较简单，还具有可读性。。。http2类比protobuf，拿到这个编码后的数据包看不出来具体是什么，需要用protobuf来解码。

**2. 多路复用**

![](https://github.com/xtaci/smux/raw/master/mux.jpg)
`图片来源: https://github.com/xtaci/smux`

在tcp的基础之上在涉及一层多路复用协议，协议包含一个streamID字段，通过streamID来标识一条流，从而在一个tcp连接之上发起多个请求-响应。多路复用并没有改变http2底层一条tcp连接的现象，使用者面向的是流而不是tcp连接，整个底层对于使用者而言是透明的。

多路复用的一个好处是可以降低tcp三次握手和四次回收造成的延时，不需要频繁的连接，断开

多路复用与http keepalive的一大同点我认为是keepalive虽然也是一条连接，但是整体是顺序的，一个(请求-响应)结束之后，下一个(请求-响应)才继续工作，上一个请求等待响应的过程是被浪费了的。

keepalive是单线程处理多个任务，多路复用是多线程处理多个任务。

**3. 服务端推送**

http2仍然需要客户端发起连接，连接成功之后发起请求，但是允许服务端返回多个响应，从网络层面而言，只要网络连接没有断开，服务端推送就是理论上可行的了，http1的短连接，服务端响应一次之后，把连接断开了，这样服务端就没有推送的能力。

### grpc的几种模式

grpc有几种模式，正常模式下，grpc客户端和服务端的流程是:

客户端发起调用 -> 服务端响应调用

但是在一些场景下，服务端需要通知客户端，grpc也是支持的，grpc除了正常模式之外，还有流模式。

流模式可以类比tcp长连接，调用一次，可以源源不断的传输数据，流模式又有两种，**单向流**和**双向流**。

所谓单向流就是指单个方向向源源不断的传输，其中单向流又可以区分为客户端单向流，服务端单向流。客户端单向流是指客户端调用服务端之后，可以源源不断的往服务端发送数据，方向反过来就是服务端单向流。

所谓双向流则是全双工，客户端和服务端都可以源源不断的传输数据，个人觉得除非对传输性能要求很高的行业，比如游戏以及网络程序，其他大部分场景都可以尝试使用grpc的双向流代替tcp长连接，帮忙解决了封包、解包、连接保活、断线重连等问题。

grpc实时推送可以采用服务端单向流的方式。

### 基于grpc server端流的实时推送
前面写了grpc以及grpc的几种模式，grpc用于实时推送正是使用了grpc的server端单向流的方式，通过编写非常少的代码即可完成grpc推送。

**server端流接口定义**
```protobuf: proto.proto
syntax = "proto3";

package grpc.push;

option go_package = "/proto";

message SubscribeReq {
    repeated string topics = 1;
}

message SubscribeReply {
    string topic = 1;
    string message = 2;
}


service PushService {
    rpc Subscribe(SubscribeReq) returns (stream SubscribeReply){}
}

```

**server实现**代码非常短，也比较简单，这是grpc实现推送的一大优势
server收到客户端请求，然后后续数据通过SendMsg方法响应回去即可

```golang: server.go
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"time"

	"github.com/ICKelin/article/books/code/broker"
	"github.com/ICKelin/article/books/code/proto"
	"google.golang.org/grpc"
)

func main() {
	srv := flag.String("l", "", "local listen addr")
	flag.Parse()

	lis, err := net.Listen("tcp", *srv)
	if err != nil {
		log.Println(err)
		return
	}
	defer lis.Close()

	server := &Server{
		b: broker.NewBroker(),
	}

	go cli(server)
	s := grpc.NewServer()
	proto.RegisterPushServiceServer(s, server)
	s.Serve(lis)
}

type Server struct {
	b *broker.Broker
}

func (s *Server) Subscribe(req *proto.SubscribeReq, stream proto.PushService_SubscribeServer) error {
	sub := &broker.Subscriber{
		Id:      time.Now().Unix(),
		Channel: make(chan *broker.PushMsg, 1024),
	}

	topics := make([]*broker.Topic, 0)
	for _, t := range req.Topics {
		topic := &broker.Topic{
			Key: t,
		}

		s.b.Subscribe(topic, sub)
		topics := append(topics, topic)
	}

	defer func() {
		for _, t := range topics {
			s.b.Unsubscribe(t, sub)
		}
	}()

	for msg := range sub.Channel {
		data, err := json.Marshal(msg.Data)
		if err != nil {
			log.Println(err)
			continue
		}

		reply := &proto.SubscribeReply{
			Topic:   msg.Topic.String(),
			Message: string(data),
		}

		err = stream.SendMsg(reply)
		if err != nil {
			log.Println(err)
			break
		}
	}

	return nil
}

func cli(s *Server) {
	tick := time.NewTicker(time.Second * 3)
	defer tick.Stop()

	for range tick.C {
		s.b.Publish(&broker.Topic{
			"test-topic",
		}, "publish msg")
	}
}

```

**客户端实现**更加简单，客户端实现只需要发起请求，然后就接数据就行了

```golang: client.go
package main

import (
	"context"
	"flag"
	"log"

	"github.com/ICKelin/article/books/code/proto"
	"google.golang.org/grpc"
)

func main() {
	srv := flag.String("r", "", "server address")
	flag.Parse()

	conn, err := grpc.Dial(*srv, grpc.WithInsecure())
	if err != nil {
		log.Println(err)
		return
	}

	cli := proto.NewPushServiceClient(conn)

	stream, err := cli.Subscribe(context.Background(), &proto.SubscribeReq{
		Topics: []string{"test-topic"},
	})
	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}

		log.Println(msg.Topic, msg.Message)
	}
}

```

## 优缺点分析

**优点**
grpc使用我归纳了下，有以下几个优点:

- 代码简单，整个推送相关的代码基本能控制在两百行以内
- 处理简单，不用考虑封包，解包，也不用考虑断线重连，框架内部都帮忙做好了。
- 清晰，使用protobuf来作为接口定义与编解码方式，非常好理解

总而言之，easy。

**缺点**
缺点需要结合具体场景进行分析，如果是手机app或者，考虑耗电，流量是否在可接受范围之内，总不能用一个grpc推送，手机热的可以烫鸡蛋来吧。


## 参考链接

[grpc.io](https://grpc.io)

[HTTP/2 简介](https://developers.google.com/web/fundamentals/performance/http2?hl=zh-cn)