package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/ICKelin/article/books/code/arq/sw"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Ltime | log.Lshortfile | log.Lmicroseconds)
	raddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:5013")
	// raddr, _ := net.ResolveUDPAddr("udp", "18.220.204.29:5013")

	client, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return
	}

	swc := sw.NewSw(client)
	go func() {
		c := 0
		tick := time.NewTimer(time.Second * 30)
		for {
			select {
			case <-tick.C:
				return
			default:
			}

			s := fmt.Sprintf("hello %d", c+1)
			c += 1
			fmt.Println("send ", s)
			swc.Send([]byte(s), raddr)
			buf, _, err := swc.Peek()
			if err != nil {
				continue
			}
			fmt.Println("recv ", string(buf))
			time.Sleep(time.Millisecond * 10)
		}
	}()
	select {}
}
