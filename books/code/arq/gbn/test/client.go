package main

import (
	"fmt"
	"net"
	"time"

	"github.com/ICKelin/article/books/code/arq/gbn"
)

func main() {
	// raddr, _ := net.ResolveUDPAddr("udp", "18.220.204.29:5012")
	raddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:5012")
	client, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return
	}

	swc := gbn.NewGbn(client, false)

	go func() {
		c := 0
		tick := time.NewTimer(time.Second * 30)
		for {
			select {
			case <-tick.C:
				return
			default:
			}

			s := fmt.Sprintf("hello %d", c)
			c += 1
			swc.Send([]byte(s), raddr)
			fmt.Println("send ", s)
			time.Sleep(time.Millisecond * 5)
			buf, _, _ := swc.Peek()
			fmt.Println("recv ", string(buf))
		}
	}()
	select {}
}
