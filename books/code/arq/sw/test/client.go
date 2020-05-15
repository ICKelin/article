package main

import (
	"fmt"
	"net"
	"time"

	"github.com/ICKelin/article/books/code/arq/sw"
)

func main() {
	raddr, _ := net.ResolveUDPAddr("udp", "18.220.204.29:5013")
	client, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return
	}

	swc := sw.NewSw(client)

	go func() {
		c := 0
		for {
			s := fmt.Sprintf("hello %d", c)
			c += 1
			swc.Send([]byte(s), raddr)
			fmt.Println("send ", s)
			time.Sleep(time.Second * 1)
		}
	}()
	select {}
}
