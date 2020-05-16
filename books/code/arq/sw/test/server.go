package main

import (
	"fmt"
	"log"
	"net"
	"sync/atomic"
	"time"

	"github.com/ICKelin/article/books/code/arq/sw"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Ltime | log.Lshortfile | log.Lmicroseconds)
	laddr, _ := net.ResolveUDPAddr("udp", ":5013")
	lis, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return
	}

	defer lis.Close()

	sws := sw.NewSw(lis)
	count := int32(0)
	go func() {
		for {
			buf, raddr, err := sws.Peek()
			if err != nil {
				continue
			}

			fmt.Println("recv ", string(buf))
			atomic.AddInt32(&count, 1)
			fmt.Println("send ", string(buf))
			sws.Send(buf, raddr)
		}
	}()

	tick := time.NewTimer(time.Second * 10)
	defer tick.Stop()
	for range tick.C {
		fmt.Println("receive count: ", count)
	}

	select {}
}
