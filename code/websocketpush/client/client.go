package main

import (
	"flag"
	"log"
	"time"

	"github.com/ICKelin/article/books/code/proto"
	"github.com/gorilla/websocket"
)

func main() {
	srv := flag.String("r", "", "server address")
	flag.Parse()

	for {
		conn, _, err := websocket.DefaultDialer.Dial("ws://"+*srv+"/ws", nil)
		if err != nil {
			log.Println(err)
			time.Sleep(time.Second * 3)
			continue
		}

		conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
		err = conn.WriteJSON(&proto.SubscribeReq{
			Topics: []string{"test-topic"},
		})
		conn.SetWriteDeadline(time.Time{})
		if err != nil {
			log.Println(err)
			time.Sleep(time.Second * 3)
			continue
		}

		go writer(conn)
		reader(conn)
		time.Sleep(time.Second * 3)
		log.Println("reconnecting")
	}
}

type replyMsg struct {
	Cmd  string      `json:"cmd"`
	Data interface{} `json:"data"`
}

func writer(conn *websocket.Conn) {
	defer conn.Close()

	tick := time.NewTicker(time.Second * 10)
	defer tick.Stop()

	hb := &replyMsg{
		Cmd: "ping",
	}

	for range tick.C {
		err := conn.WriteJSON(hb)
		if err != nil {
			log.Println(err)
			break
		}
		log.Println("[D] write heartbeat to server")
	}
}

func reader(conn *websocket.Conn) {
	var obj replyMsg
	for {
		err := conn.ReadJSON(&obj)
		if err != nil {
			log.Println(err)
			return
		}

		if obj.Cmd == "push" {
			log.Println(obj.Data)
		}
	}
}
