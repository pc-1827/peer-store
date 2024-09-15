package main

import (
	"fmt"
	"log"

	"github.com/pc-1827/distributed-file-system/p2p"
)

func main() {
	TCPTransport := p2p.NewTCPTransport(":3000")

	if err := TCPTransport.ListenAndAccept(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Hello, World!")

	select {}
}
