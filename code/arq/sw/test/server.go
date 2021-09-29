package main

import (
	"fmt"
	"log"
	"net"

	"github.com/ICKelin/article/books/code/arq/sw"
)

type server struct {
	sws      *sw.Sw
	peerAddr *net.UDPAddr
}

func (s *server) listenAndServe() {
	laddr, _ := net.ResolveUDPAddr("udp", ":5013")
	lis, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return
	}

	defer lis.Close()

	sws := sw.NewSw(
		func(buf []byte) {
			_, err := lis.WriteTo(buf, s.peerAddr)
			if err != nil {
				fmt.Println(err)
			}
		})

	s.sws = sws

	// 读udp数据
	// 通过sws.Input输入sw协议解码
	// 通过sws.RecvFrom读取解码后的数据
	go func() {
		buf := make([]byte, 2048)
		for {
			nr, raddr, err := lis.ReadFromUDP(buf)
			if err != nil {
				fmt.Println("read from udp:", err)
				break
			}

			if s.peerAddr == nil {
				s.peerAddr = raddr
			}

			if s.peerAddr.String() != raddr.String() {
				fmt.Println("not match client")
				continue
			}

			sws.Input(buf[:nr])
		}
	}()

	// 读
	go func() {
		for {
			buf, err := sws.RecvFrom()
			if err != nil {
				fmt.Println(err)
				continue
			}

			fmt.Println("server recv from: ", string(buf))

			// 回显
			sws.SendTo(buf)
		}
	}()

	select {}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Ltime | log.Lshortfile | log.Lmicroseconds)
	s := &server{}
	s.listenAndServe()
}
