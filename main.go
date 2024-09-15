package main

import (
	"fmt"
	"log"

	"github.com/pc-1827/distributed-file-system/p2p"
)

func main() {
	tcpOpts := p2p.TCPTransportOptions{
		ListenAddress: ":3000",
		HandShakeFunc: p2p.NOTHandShakeFunc,
		Decoder:       p2p.DefaultDecoder{},
	}
	tcpTransport := p2p.NewTCPTransport(tcpOpts)

	if err := tcpTransport.ListenAndAccept(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Hello, World!")

	select {}
}
