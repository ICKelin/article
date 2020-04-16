package main

import (
	"context"
	"flag"
	"log"
	"time"

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

	for {
		stream, err := cli.Subscribe(context.Background(), &proto.SubscribeReq{
			Topics: []string{"test-topic"},
		})
		if err != nil {
			log.Println(err)
			time.Sleep(time.Second * 1)
			continue
		}

		for {
			msg, err := stream.Recv()
			if err != nil {
				break
			}

			log.Println(msg.Topic, msg.Message)
		}
		time.Sleep(time.Second * 1)
		log.Println("reconnecting")
	}
}
