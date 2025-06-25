package main

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/pc-1827/peer-store/p2p"
)

func makeChordServer(listenAddress string) *FileServer {
	tcpTransportOptions := p2p.TCPTransportOptions{
		ListenAddress: listenAddress,
		HandShakeFunc: p2p.NOTHandShakeFunc,
		Decoder:       p2p.DefaultDecoder{},
	}
	tcpTransport := p2p.NewTCPTransport(tcpTransportOptions)

	FileServerOptions := FileServerOptions{
		StorageRoot:       listenAddress + "_chord_network",
		PathTransformFunc: CASPathTransformFunc,
		Transport:         tcpTransport,
	}

	server := NewFileServer(FileServerOptions)
	tcpTransport.OnPeer = server.OnPeer
	return server
}


func TestChordNetworkBasic(t *testing.T) {
	// Create servers
	server1 := makeChordServer("127.0.0.1:5001")
	server2 := makeChordServer("127.0.0.1:5002")
	server3 := makeChordServer("127.0.0.1:5003")
	server4 := makeChordServer("127.0.0.1:5004")

	// Start servers
	go server1.Start()
	time.Sleep(200 * time.Millisecond)
	go server2.Start()
	time.Sleep(200 * time.Millisecond)
	go server3.Start()
	time.Sleep(200 * time.Millisecond)
	go server4.Start()
	time.Sleep(500 * time.Millisecond)

	// Initialize Chord on the first node
	server1.InitChord()
	time.Sleep(500 * time.Millisecond)

	// Join the Chord ring
	err := server2.JoinChord(server1.Transport.Addr())
	if err != nil {
		t.Fatalf("Failed to join Chord ring: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	err = server3.JoinChord(server1.Transport.Addr())
	if err != nil {
		t.Fatalf("Failed to join Chord ring: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	err = server4.JoinChord(server2.Transport.Addr())
	if err != nil {
		t.Fatalf("Failed to join Chord ring: %v", err)
	}
	time.Sleep(1 * time.Second)

	// Create test file
	testFile := []byte("This is a test file for the Chord DHT")
	fileName := "chord_test.txt"

	// Store the file through node 1
	cid, err := server1.StoreChord(fileName, bytes.NewReader(testFile))
	if err != nil {
		t.Fatalf("Failed to store file: %v", err)
	}
	t.Logf("Stored file with CID: %s", cid)
	time.Sleep(500 * time.Millisecond)

	// Retrieve the file through node 3
	reader, name, err := server3.GetChord(cid)
	if err != nil {
		t.Fatalf("Failed to retrieve file: %v", err)
	}
	if name != fileName {
		t.Errorf("Filename mismatch: expected %s, got %s", fileName, name)
	}

	// Check file content
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to read file data: %v", err)
	}
	if !bytes.Equal(data, testFile) {
		t.Errorf("File content mismatch: expected %q, got %q", testFile, data)
	}

	// Delete the file through node 4
	err = server4.DeleteChord(cid)
	if err != nil {
		t.Fatalf("Failed to delete file: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Try to retrieve the deleted file
	_, _, err = server2.GetChord(cid)
	if err == nil {
		t.Errorf("Expected error when retrieving deleted file")
	}

	// Clean up
	server1.Stop()
	server2.Stop()
	server3.Stop()
	server4.Stop()
	time.Sleep(200 * time.Millisecond)
}
