package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/ICKelin/article/books/code/arq/gbn"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Ltime | log.Lshortfile | log.Lmicroseconds)
	raddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:5012")
	// raddr, _ := net.ResolveUDPAddr("udp", "192.168.31.65:5013")

	client, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return
	}

	swc := gbn.NewGbn(
		func(buf []byte) {
			_, err := client.Write(buf)
			if err != nil {
				fmt.Println(err)
				return
			}

		}, true)

	// 写
	go func() {
		c := 0
		tick := time.NewTimer(time.Second * 10)
		for {
			select {
			case <-tick.C:
				return
			default:
			}

			s := fmt.Sprintf("hello %d", c+1)
			c += 1
			swc.SendTo([]byte(s))
		}
	}()

	// 读
	go func() {
		for {
			buf, err := swc.RecvFrom()
			if err != nil {
				fmt.Println(err)
				continue
			}
			fmt.Println("recv ", string(buf))
		}
	}()

	// udp读包
	// 输入sw.Input进行协议解码
	// 通过swc.RecvFrom读取解码之后的缓冲区数据
	go func() {
		for {
			buf := make([]byte, 2048)
			nr, _, err := client.ReadFromUDP(buf)
			if err != nil {
				fmt.Println(err)
				break
			}

			swc.Input(buf[:nr])
		}
	}()

	select {}
}
