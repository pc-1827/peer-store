package main

import (
	"fmt"
	"io"
	"log"
	"os"
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

	for i := 0; i < 1; i++ {
		// key := fmt.Sprintf("random_picture_%d.jpeg", i)
		// data_string := "random_bullshit"
		// for i := 0; i < 100; i++ {
		// 	data_string = data_string + fmt.Sprintf("abcdefghijklmnopqrstuvwxyz_%d_random_bullshit", i)
		// }
		// data := bytes.NewReader([]byte(data_string))

		filePath := "test_files/big-file"
		file, err := os.Open(filePath)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		key := "test-6.png"
		server3.Store(key, file)
		time.Sleep(5 * time.Millisecond)

		if err := server3.store.Delete(server3.ID, key); err != nil {
			log.Fatal(err)
		}

		r, err := server3.Get(key)
		if err != nil {
			log.Fatal(err)
		}

		// if err := server3.Remove(key); err != nil {
		// 	log.Fatal(err)
		// }

		b, err := io.ReadAll(r)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Length of string is: %d\n", len(b))
		//fmt.Println(string(b))
	}
}
