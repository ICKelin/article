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
	// raddr, _ := net.ResolveUDPAddr("udp", "192.168.31.65:5013")

	client, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return
	}

	swc := sw.NewSw(
		func(buf []byte) {
			fmt.Println("output ", buf)
			_, err := client.Write(buf)
			if err != nil {
				fmt.Println(err)
				return
			}

		})

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
			swc.Output([]byte(s))
		}
	}()

	go func() {
		buf := make([]byte, 2048)
		for {
			nr, _, err := client.ReadFromUDP(buf)
			if err != nil {
				fmt.Println(err)
				break
			}

			swc.Input(buf[:nr])
			fmt.Println("inputed ", buf[:nr])
		}
	}()

	go swc.Stat()
	for {
		buf, err := swc.RecvFrom()
		if err != nil {
			fmt.Println(err)
			continue
		}
		fmt.Println("recv ", string(buf))
	}
	select {}
}
