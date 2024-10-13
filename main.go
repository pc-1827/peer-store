package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/pc-1827/distributed-file-system/p2p"
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
		//key := fmt.Sprintf("random_picture_%d1.jpeg", i)
		key2 := fmt.Sprintf("random_picture_%d.txt", i)
		data_string := "random_bullshit"
		for j := 0; j < 1; j++ {
			data_string = data_string + fmt.Sprintf("abcdefghijklmnopqrstuvwxyz_%d_random_bullshit", i)
		}
		data := bytes.NewReader([]byte(data_string))

		filePath := "test_files/Happy Life FREDJI (No Copyright Music).mp3"
		file, err := os.Open(filePath)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		time.Sleep(100 * time.Millisecond)

		key := "test123.mp3"
		cid, err := server3.Store(key, file)
		time.Sleep(500 * time.Millisecond)
		server2.Store(key2, data)
		if err != nil {
			log.Fatal(err)
		}
		time.Sleep(500 * time.Millisecond)

		if err := server3.store.Delete(cid); err != nil {
			log.Fatal(err)
		}

		r, fileName, err := server3.Get(cid)
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

		if err := server3.Remove(cid); err != nil {
			log.Fatal(err)
		}

		b, err := io.ReadAll(r)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Length of string is: %d\n", len(b))
		//fmt.Println(string(b))
	}
	select {}
}

// func main() {
// 	// server1 := makeServer(":3000", "")
// 	// server2 := makeServer(":4000", "")
// 	// server3 := makeServer(":5000", ":3000", ":4000")
// 	server1 := makeServer("127.0.0.1:3000")
// 	server2 := makeServer("127.0.0.1:4000")
// 	server3 := makeServer("127.0.0.1:5000")
// 	server4 := makeServer("127.0.0.1:6000")
// 	go func() { log.Fatal(server1.Start()) }()
// 	time.Sleep(500 * time.Millisecond)
// 	go func() { log.Fatal(server2.Start()) }()
// 	time.Sleep(500 * time.Millisecond)
// 	go func() { log.Fatal(server3.Start()) }()
// 	time.Sleep(500 * time.Millisecond)
// 	go func() { log.Fatal(server4.Start()) }()

// 	time.Sleep(1 * time.Second)
// 	// go server3.Start()
// 	// time.Sleep(1 * time.Second)

// 	server3.Add("127.0.0.1:3000")
// 	time.Sleep(500 * time.Millisecond)
// 	server1.Add("127.0.0.1:4000")
// 	time.Sleep(500 * time.Millisecond)
// 	server2.Add("127.0.0.1:6000")

// 	for i := 0; i < 10; i++ {
// 		key := fmt.Sprintf("random_picture_%d1.jpeg", i)
// 		key2 := fmt.Sprintf("random_picture_%d.jpeg", i)
// 		data_string := "random_bullshit"
// 		for i := 0; i < 1; i++ {
// 			data_string = data_string + fmt.Sprintf("abcdefghijklmnopqrstuvwxyz_%d_random_bullshit", i)
// 		}
// 		data := bytes.NewReader([]byte(data_string))

// 		filePath := "test_files/Happy Life FREDJI (No Copyright Music).mp3"
// 		file, err := os.Open(filePath)
// 		if err != nil {
// 			log.Fatal(err)
// 		}
// 		defer file.Close()

// 		// key := "test-6.png"
// 		server3.Store(key, file)
// 		time.Sleep(5 * time.Millisecond)
// 		server4.Store(key2, data)
// 		time.Sleep(5 * time.Millisecond)

// 		if err := server2.store.Delete(server2.ID, key); err != nil {
// 			log.Fatal(err)
// 		}

// 		_, err = server2.Get(key)
// 		if err != nil {
// 			log.Fatal(err)
// 		}

// 		r, err := server1.Get(key2)
// 		if err != nil {
// 			log.Fatal(err)
// 		}

// 		if err := server3.Remove(key); err != nil {
// 			log.Fatal(err)
// 		}

// 		b, err := io.ReadAll(r)
// 		if err != nil {
// 			log.Fatal(err)
// 		}

// 		fmt.Printf("Length of string is: %d\n", len(b))
// 		//fmt.Println(string(b))
// 	}
// 	select {}
// }
