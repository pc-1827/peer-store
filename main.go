package main

import (
	"log"
	"time"

	"github.com/pc-1827/distributed-file-system/p2p"
)

func main() {
	tcpTransportOptions := p2p.TCPTransportOptions{
		ListenAddress: ":3000",
		HandShakeFunc: p2p.NOTHandShakeFunc,
		Decoder:       p2p.DefaultDecoder{},
		// OnPeer func
	}
	tcpTransport := p2p.NewTCPTransport(tcpTransportOptions)

	FileServerOptions := FileServerOptions{
		StorageRoot:       ":3000_network",
		PathTransformFunc: CASPathTransformFunc,
		Transport:         tcpTransport,
	}

	s := NewFileServer(FileServerOptions)

	go func() {
		time.Sleep(time.Second * 5)
		s.Stop()
	}()

	if err := s.Start(); err != nil {
		log.Fatal(err)
	}
}

// func OnPeer(peer p2p.Peer) error {
// 	//return fmt.Errorf("failed to onpeer connection")
// 	peer.Close()
// 	//fmt.Print("Peer connected\n")
// 	return nil
// }

// func main() {
// 	tcpOpts := p2p.TCPTransportOptions{
// 		ListenAddress: ":3000",
// 		HandShakeFunc: p2p.NOTHandShakeFunc,
// 		Decoder:       p2p.DefaultDecoder{},
// 		OnPeer:        OnPeer,
// 	}
// 	tcpTransport := p2p.NewTCPTransport(tcpOpts)

// 	go func() {
// 		for {
// 			msg := <-tcpTransport.Consume()
// 			fmt.Printf("Received message from %v: %v\n", msg.From, string(msg.Payload))
// 		}
// 	}()

// 	if err := tcpTransport.ListenAndAccept(); err != nil {
// 		log.Fatal(err)
// 	}
// 	fmt.Println("Hello, World!")

// 	select {}
// }
