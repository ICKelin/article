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

	go func() {
		buf := make([]byte, 2048)
		for {
			nr, raddr, err := lis.ReadFromUDP(buf)
			if err != nil {
				fmt.Println(err)
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

	for {
		buf, err := sws.RecvFrom()
		if err != nil {
			fmt.Println(err)
			continue
		}

		fmt.Println("server recv from: ", string(buf))
		sws.Output(buf)
	}
}

func (s *server) read(buf []byte) {
}

func main() {
	log.SetFlags(log.LstdFlags | log.Ltime | log.Lshortfile | log.Lmicroseconds)
	s := &server{}
	s.listenAndServe()
	select {}
}
