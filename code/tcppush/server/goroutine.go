package main

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/ICKelin/article/books/code/broker"
	"github.com/ICKelin/article/books/code/proto"
	"github.com/ICKelin/article/books/code/tcppush/codec"
)

type writeReq struct {
	cmd    int
	body   []byte
	result chan error
}

type GServer struct {
	addr string
	b    *broker.Broker
}

func NewGServer(addr string) *GServer {
	return &GServer{
		addr: addr,
		b:    broker.NewBroker(),
	}
}

func (s *GServer) ListenAndServe() error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	for {
		conn, err := lis.Accept()
		if err != nil {
			return err
		}

		go s.onConn(conn)
	}
}

func (s *GServer) onConn(conn net.Conn) {
	defer conn.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := proto.SubscribeReq{}
	err := codec.ReadJSON(conn, &req)
	if err != nil {
		log.Println(err)
		return
	}

	// 订阅
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
		topics = append(topics, topic)
	}

	defer func() {
		for _, t := range topics {
			s.b.Unsubscribe(t, sub)
		}
	}()

	// 控制信令
	sndbuf := make(chan *writeReq)
	go s.reader(ctx, conn, sndbuf)
	s.writer(ctx, conn, sub, sndbuf)
}

func (s *GServer) reader(ctx context.Context, conn net.Conn, sndbuf chan *writeReq) {
	defer conn.Close()

	hbreq := &writeReq{
		cmd:    codec.CmdHeartbeat,
		result: make(chan error),
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		hdr, _, err := codec.Read(conn)
		if err != nil {
			log.Println(err)
			break
		}

		switch hdr.Cmd() {
		case codec.CmdHeartbeat:
			select {
			case sndbuf <- hbreq:
				select {
				case err := <-hbreq.result:
					if err != nil {
						log.Println("write heartbeat fail: ", err)
					}
				}
			default:
			}
		default:
			log.Println("unsupported cmd: ", hdr.Cmd())
		}
	}
}

func (s *GServer) writer(ctx context.Context, conn net.Conn, sub *broker.Subscriber, sndbuf chan *writeReq) {
	defer conn.Close()
	for {
		select {
		case <-ctx.Done():
			return

		case req := <-sndbuf:
			log.Println("[D] s2c heartbeat")
			err := codec.Write(conn, req.cmd, req.body)
			req.result <- err

		case msg := <-sub.Channel:
			log.Println("[D] ", msg.Data)
			err := codec.WriteJSON(conn, codec.CmdData, msg)
			if err != nil {
				log.Println(err)
			}
		}
	}
}
