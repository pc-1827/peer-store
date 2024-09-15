package main

import (
	"fmt"
	"log"

	"github.com/pc-1827/distributed-file-system/p2p"
)

func OnPeer(peer p2p.Peer) error {
	//return fmt.Errorf("failed to onpeer connection")
	peer.Close()
	//fmt.Print("Peer connected\n")
	return nil
}

func main() {
	tcpOpts := p2p.TCPTransportOptions{
		ListenAddress: ":3000",
		HandShakeFunc: p2p.NOTHandShakeFunc,
		Decoder:       p2p.DefaultDecoder{},
		OnPeer:        OnPeer,
	}
	tcpTransport := p2p.NewTCPTransport(tcpOpts)

	go func() {
		for {
			msg := <-tcpTransport.Consume()
			fmt.Printf("Received message from %v: %v\n", msg.From, string(msg.Payload))
		}
	}()

	if err := tcpTransport.ListenAndAccept(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Hello, World!")

	select {}
}
