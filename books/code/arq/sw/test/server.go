package main

import (
	"fmt"
	"net"

	"github.com/ICKelin/article/books/code/arq/sw"
)

func main() {
	laddr, _ := net.ResolveUDPAddr("udp", ":5013")
	lis, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return
	}

	defer lis.Close()

	sws := sw.NewSw(lis)

	go func() {
		for {
			buf, _ := sws.Peek()
			fmt.Println(string(buf))
		}
	}()

	select {}
}
