package main

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/ICKelin/article/books/code/arq/gbn"
)

func main() {
	laddr, _ := net.ResolveUDPAddr("udp", ":5012")
	lis, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return
	}

	defer lis.Close()

	sws := gbn.NewGbn(lis, false)
	count := int32(0)
	go func() {
		for {
			buf, raddr, _ := sws.Peek()
			fmt.Println("recv", string(buf))
			atomic.AddInt32(&count, 1)
			sws.Send(buf, raddr)
			fmt.Println("send ", string(buf))
		}
	}()

	tick := time.NewTimer(time.Second * 10)
	defer tick.Stop()
	for range tick.C {
		fmt.Println("receive count: ", count)
	}

	select {}
}
