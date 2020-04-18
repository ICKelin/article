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
