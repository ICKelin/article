package main

import (
	"fmt"
	"net"

	"github.com/ICKelin/article/books/code/arq/gbn"
)

func main() {
	laddr, _ := net.ResolveUDPAddr("udp", ":5012")
	lis, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return
	}

	defer lis.Close()

	sws := gbn.NewGbn(lis)

	go func() {
		for {
			buf, _ := sws.Peek()
			fmt.Println(string(buf))
		}
	}()

	select {}
}
