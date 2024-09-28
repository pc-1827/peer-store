package main

import (
	"bytes"
	"log"
	"time"

	"github.com/pc-1827/distributed-file-system/p2p"
)

func makeServer(listenAddress string, nodes ...string) *FileServer {
	tcpTransportOptions := p2p.TCPTransportOptions{
		ListenAddress: listenAddress,
		HandShakeFunc: p2p.NOTHandShakeFunc,
		Decoder:       p2p.DefaultDecoder{},
	}
	tcpTransport := p2p.NewTCPTransport(tcpTransportOptions)

	FileServerOptions := FileServerOptions{
		StorageRoot:       listenAddress + "_network",
		PathTransformFunc: CASPathTransformFunc,
		Transport:         tcpTransport,
		BootstrapNodes:    nodes,
	}

	server := NewFileServer(FileServerOptions)
	tcpTransport.OnPeer = server.OnPeer
	return server
}

func main() {
	server1 := makeServer(":3000", "")
	server2 := makeServer(":4000", ":3000")
	go func() {
		log.Fatal(server1.Start())
	}()

	time.Sleep(1 * time.Second)
	go server2.Start()
	time.Sleep(1 * time.Second)

	data := bytes.NewReader([]byte("My big data file here!"))

	server2.StoreData("bigdata", data)

	select {}
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
