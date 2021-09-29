package main

import (
	"log"
	"time"

	"github.com/ICKelin/article/books/code/broker"
)

func main() {
	s := NewGServer(":1234")
	go cli(s.b)
	log.Println(s.ListenAndServe())
}

func cli(b *broker.Broker) {
	tick := time.NewTicker(time.Second * 3)
	defer tick.Stop()

	for range tick.C {
		b.Publish(&broker.Topic{
			Key: "test-topic",
		}, "publish msg")
	}
}
