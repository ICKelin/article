package main

import (
	"log"
	"net/http"
	"time"

	"github.com/ICKelin/article/books/code/broker"
	"github.com/ICKelin/article/books/code/proto"
	"github.com/gorilla/websocket"
)

func main() {
	b := broker.NewBroker()
	go cli(b)

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 1024*64, 1024*64)
		if err != nil {
			log.Println(err)
			return
		}

		handleConn(b, conn)
	})

	http.ListenAndServe(":8091", nil)
}

func handleConn(b *broker.Broker, conn *websocket.Conn) {
	defer conn.Close()

	req := proto.SubscribeReq{}
	err := conn.ReadJSON(&req)
	if err != nil {
		log.Println(err)
		return
	}

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

	sndqueue := make(chan interface{})
	go reader(conn, sndqueue)
	writer(conn, sub, sndqueue)
}

type replyMsg struct {
	Cmd  string      `json:"cmd"`
	Data interface{} `json:"data"`
}

func reader(conn *websocket.Conn, sndqueue chan interface{}) {
	defer conn.Close()

	hb := replyMsg{
		Cmd: "pong",
	}
	for {
		err := conn.ReadJSON(&hb)
		if err != nil {
			log.Println(err)
			break
		}

		if hb.Cmd == "ping" {
			select {
			case sndqueue <- &hb:
			default:
			}
		}
	}
}

func writer(conn *websocket.Conn, sub *broker.Subscriber, sndqueue chan interface{}) {
	defer conn.Close()

	for {
		select {
		case msg := <-sub.Channel:
			reply := &replyMsg{
				Cmd:  "push",
				Data: msg.Data,
			}

			conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
			err := conn.WriteJSON(reply)
			conn.SetWriteDeadline(time.Time{})
			if err != nil {
				log.Println(err)
				return
			}

		case msg := <-sndqueue:
			log.Println("[D] write heartbeat to client")
			conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
			err := conn.WriteJSON(msg)
			conn.SetWriteDeadline(time.Time{})
			if err != nil {
				log.Println(err)
				return
			}
		}
	}
}

func cli(b *broker.Broker) {
	tick := time.NewTicker(time.Second * 3)
	defer tick.Stop()

	for range tick.C {
		b.Publish(&broker.Topic{
			"test-topic",
		}, "publish msg")
	}
}
