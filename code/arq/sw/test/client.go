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
			fmt.Println("send ", s)
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

	select {}
}
