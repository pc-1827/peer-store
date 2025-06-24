package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/pc-1827/peer-store/p2p"
)

// func makeServer(listenAddress string, nodes ...string) *FileServer {
func makeServer(listenAddress string) *FileServer {
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
	}

	server := NewFileServer(FileServerOptions)
	tcpTransport.OnPeer = server.OnPeer
	return server
}

func main() {
	// server1 := makeServer(":3000", "")
	// server2 := makeServer(":4000", "")
	// server3 := makeServer(":5000", ":3000", ":4000")
	server1 := makeServer("127.0.0.1:3000")
	server2 := makeServer("127.0.0.1:4000")
	server3 := makeServer("127.0.0.1:5000")
	server4 := makeServer("127.0.0.1:6000")
	go func() { log.Fatal(server1.Start()) }()
	time.Sleep(500 * time.Millisecond)
	go func() { log.Fatal(server2.Start()) }()
	time.Sleep(500 * time.Millisecond)
	go func() { log.Fatal(server3.Start()) }()
	time.Sleep(500 * time.Millisecond)
	go func() { log.Fatal(server4.Start()) }()

	time.Sleep(1 * time.Second)
	// go server3.Start()
	// time.Sleep(1 * time.Second)

	server3.Add("127.0.0.1:3000")
	time.Sleep(500 * time.Millisecond)
	server1.Add("127.0.0.1:4000")
	time.Sleep(500 * time.Millisecond)
	server2.Add("127.0.0.1:6000")

	for i := 0; i < 1; i++ {
		// key2 := "test.txt"
		// data_string := "random data for testing"
		// data := bytes.NewReader([]byte(data_string))

		filePath := "test_files/Anonymised_Resume.pdf"
		file, err := os.Open(filePath)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		time.Sleep(100 * time.Millisecond)

		key := "test123.pdf"
		cid, _ := server3.Store(key, file)
		time.Sleep(500 * time.Millisecond)
		// server2.Store(key2, data)
		// if err != nil {
		// 	log.Fatal(err)
		// }
		// time.Sleep(500 * time.Millisecond)

		// if err := server3.store.Delete(cid); err != nil {
		// 	log.Fatal(err)
		// }

		r, fileName, err := server1.Get(cid)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("FileName: %q\n", fileName)

		if err := os.MkdirAll("test_output", os.ModePerm); err != nil {
			log.Fatal(err)
		}

		f, err := os.Create(fmt.Sprintf("test_output/%s", fileName))
		if err != nil {
			log.Fatal(err)
		}

		io.Copy(f, r)

		// r, err := server1.Get(key2)
		// if err != nil {
		// 	log.Fatal(err)
		// }

		// if err := server3.Remove(cid); err != nil {
		// 	log.Fatal(err)
		// }

		b, err := io.ReadAll(r)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Length of string is: %d\n", len(b))
		//fmt.Println(string(b))
	}
	select {}
}


