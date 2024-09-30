package main

import (
	"bytes"
	"fmt"
	"io"
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
		EncKey:            newEncryptionKey(),
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
	server2 := makeServer(":4000", "")
	server3 := makeServer(":5000", ":3000", ":4000")
	go func() { log.Fatal(server1.Start()) }()
	time.Sleep(500 * time.Millisecond)
	go func() { log.Fatal(server2.Start()) }()

	time.Sleep(1 * time.Second)
	go server3.Start()
	time.Sleep(1 * time.Second)

	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("random_picture_%d.jpeg", i)
		data := bytes.NewReader([]byte("My big data file here!"))

		server3.Store(key, data)
		time.Sleep(5 * time.Millisecond)

		if err := server3.store.Delete(server3.ID, key); err != nil {
			log.Fatal(err)
		}

		r, err := server3.Get(key)
		if err != nil {
			log.Fatal(err)
		}

		b, err := io.ReadAll(r)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(string(b))
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
