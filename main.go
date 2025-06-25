package main

import (
    "fmt"
    "log"
    "time"

    "github.com/pc-1827/peer-store/p2p"
)

func init() {
    // Print p2p protocol constants for debugging
    log.Printf("P2P Protocol Constants - IncomingMessage: %d, IncomingStream: %d",
        p2p.IncomingMessage, p2p.IncomingStream)
    
    // Register Gob types
    registerGobTypes()
}

func main() {
    // Parse config from command-line arguments
    config, err := ParseConfig()
    if err != nil {
        log.Fatalf("Error parsing config: %v", err)
    }

    // Create and start the server
    _, err = StartServer(config)
    if err != nil {
        log.Fatalf("Error starting server: %v", err)
    }

    // Wait indefinitely
    log.Printf("Server running at %s with architecture %s", 
        config.ListenAddr, config.Architecture)
    log.Printf("API server running at http://localhost:%d", config.APIPort)
    
    // Block main goroutine
    select {}
}

func StartServer(config *Config) (*FileServer, error) {
    // Create transport
    tcpTransportOptions := p2p.TCPTransportOptions{
        ListenAddress: config.ListenAddr,
        HandShakeFunc: p2p.NOTHandShakeFunc,
        Decoder:       p2p.DefaultDecoder{},
    }
    tcpTransport := p2p.NewTCPTransport(tcpTransportOptions)

    // Create file server options
    pathTransform := CASPathTransformFunc // Use CAS for both architectures
    
    fileServerOptions := FileServerOptions{
        StorageRoot:       config.ListenAddr + "_" + config.Architecture + "_network",
        PathTransformFunc: pathTransform,
        Transport:         tcpTransport,
        Config:            config, // Pass the config to the server
    }

    // Create server
    server := NewFileServer(fileServerOptions)
    tcpTransport.OnPeer = server.OnPeer

    // Initialize network configuration (for mesh)
    InitializeNetwork(config)

    // Start the server (will also start API server)
    go func() {
        if err := server.Start(); err != nil {
            log.Fatalf("Server error: %v", err)
        }
    }()
    
    // Give the server time to start
    time.Sleep(1 * time.Second)

    // Initialize node based on architecture
    if config.Architecture == "chord" {
        if config.JoinAddr == "" {
            // Create a new Chord ring
            server.InitChord()
            log.Printf("Created new Chord ring at %s", config.ListenAddr)
        } else {
            // Join existing Chord ring
            err := server.JoinChord(config.JoinAddr)
            if err != nil {
                return nil, fmt.Errorf("failed to join Chord ring at %s: %w", 
                    config.JoinAddr, err)
            }
            log.Printf("Joined Chord ring at %s", config.JoinAddr)
        }
    } else if config.Architecture == "mesh" {
        if config.JoinAddr != "" {
            // Join existing mesh network
            err := server.Add(config.JoinAddr)
            if err != nil {
                return nil, fmt.Errorf("failed to join mesh network at %s: %w", 
                    config.JoinAddr, err)
            }
            log.Printf("Joined mesh network at %s", config.JoinAddr)
        } else {
            log.Printf("Created new mesh network at %s", config.ListenAddr)
        }
    }

    return server, nil
}

// func init() {
// 	// Print p2p protocol constants for debugging
// 	log.Printf("P2P Protocol Constants - IncomingMessage: %d, IncomingStream: %d",
// 		p2p.IncomingMessage, p2p.IncomingStream)
// }

// // func makeServer(listenAddress string, nodes ...string) *FileServer {
// // func makeServer(listenAddress string) *FileServer {
// // 	tcpTransportOptions := p2p.TCPTransportOptions{
// // 		ListenAddress: listenAddress,
// // 		HandShakeFunc: p2p.NOTHandShakeFunc,
// // 		Decoder:       p2p.DefaultDecoder{},
// // 	}
// // 	tcpTransport := p2p.NewTCPTransport(tcpTransportOptions)

// // 	FileServerOptions := FileServerOptions{
// // 		StorageRoot:       listenAddress + "_network",
// // 		PathTransformFunc: CASPathTransformFunc,
// // 		Transport:         tcpTransport,
// // 	}

// // 	server := NewFileServer(FileServerOptions)
// // 	tcpTransport.OnPeer = server.OnPeer
// // 	return server
// // }

// func main() {
// 	// Choose the test to run
// 	if len(os.Args) > 1 && os.Args[1] == "direct" {
// 		TestDirectFileTransfer()
// 	} else {
// 		ExampleChordNetwork()
// 	}
// }

// func makeChordServer(listenAddress string) *FileServer {
// 	tcpTransportOptions := p2p.TCPTransportOptions{
// 		ListenAddress: listenAddress,
// 		HandShakeFunc: p2p.NOTHandShakeFunc,
// 		Decoder:       p2p.DefaultDecoder{},
// 	}
// 	tcpTransport := p2p.NewTCPTransport(tcpTransportOptions)

// 	FileServerOptions := FileServerOptions{
// 		StorageRoot:       listenAddress + "_chord_network",
// 		PathTransformFunc: CASPathTransformFunc,
// 		Transport:         tcpTransport,
// 	}

// 	server := NewFileServer(FileServerOptions)
// 	tcpTransport.OnPeer = server.OnPeer
// 	return server
// }

// // Update the ExampleChordNetwork function (timing part)

// func ExampleChordNetwork() {
// 	// Create servers
// 	server1 := makeChordServer("127.0.0.1:3000")
// 	server2 := makeChordServer("127.0.0.1:4000")

// 	// Start servers
// 	go func() { log.Fatal(server1.Start()) }()
// 	time.Sleep(1 * time.Second)
// 	go func() { log.Fatal(server2.Start()) }()
// 	time.Sleep(1 * time.Second)

// 	// Create Chord ring with first node
// 	server1.InitChord()
// 	fmt.Println("Node 1 created Chord ring")
// 	time.Sleep(2 * time.Second)

// 	// Print initial state
// 	printNodeState(server1, "1")

// 	// Join the Chord ring with second node
// 	err := server2.JoinChord(server1.Transport.Addr())
// 	if err != nil {
// 		log.Fatalf("Failed to join ring: %v", err)
// 	}
// 	fmt.Println("Node 2 joined Chord ring")
// 	time.Sleep(3 * time.Second) // Allow more time for stabilization

// 	// Print node states after joining
// 	printNodeState(server1, "1")
// 	printNodeState(server2, "2")

// 	// Store a very simple string to isolate the storage issue
// 	fileContent := "Test content"
// 	log.Printf("Storing file with content: %s", fileContent)

// 	// Add timeout for store operation
// 	storeDone := make(chan string)
// 	storeErr := make(chan error)

// 	go func() {
// 		cid, err := server1.StoreChord("test.txt", bytes.NewReader([]byte(fileContent)))
// 		if err != nil {
// 			storeErr <- err
// 			return
// 		}
// 		storeDone <- cid
// 	}()

// 	var cid string
// 	select {
// 	case err := <-storeErr:
// 		log.Fatalf("Failed to store file: %v", err)
// 	case cid = <-storeDone:
// 		fmt.Printf("Stored file with CID: %s\n", cid)
// 	case <-time.After(10 * time.Second):
// 		log.Fatalf("Timeout storing file")
// 	}

// 	time.Sleep(2 * time.Second)

// 	// Check which node has the file
// 	hasFile1 := server1.store.Has(cid)
// 	hasFile2 := server2.store.Has(cid)
// 	log.Printf("File presence - Node 1: %v, Node 2: %v", hasFile1, hasFile2)

// 	// Try to retrieve the file with timeout
// 	log.Printf("Attempting to retrieve file %s from Node 2", cid)

// 	retrieveDone := make(chan struct{})
// 	retrieveErr := make(chan error)
// 	var retrievedData []byte
// 	var fileName string

// 	go func() {
// 		reader, name, err := server2.GetChord(cid)
// 		if err != nil {
// 			retrieveErr <- err
// 			return
// 		}

// 		data, err := io.ReadAll(reader)
// 		if err != nil {
// 			retrieveErr <- err
// 			return
// 		}

// 		retrievedData = data
// 		fileName = name
// 		retrieveDone <- struct{}{}
// 	}()

// 	select {
// 	case err := <-retrieveErr:
// 		log.Fatalf("Failed to retrieve file: %v", err)
// 	case <-retrieveDone:
// 		fmt.Printf("Retrieved file with name: %s\n", fileName)
// 		fmt.Printf("Retrieved content: %s\n", string(retrievedData))

// 		if string(retrievedData) == fileContent {
// 			fmt.Println("SUCCESS: Retrieved content matches original")
// 		} else {
// 			fmt.Printf("ERROR: Content mismatch - expected '%s', got '%s'\n",
// 				fileContent, string(retrievedData))
// 		}
// 	case <-time.After(10 * time.Second):
// 		log.Fatalf("Timeout retrieving file")
// 	}

// 	// Clean up
// 	time.Sleep(1 * time.Second)
// 	server1.Stop()
// 	server2.Stop()
// 	fmt.Println("Chord test completed")
// }

// func printNodeState(server *FileServer, nodeID string) {
// 	node := server.chordNode
// 	predAddr := "nil"
// 	if node.predecessor != nil {
// 		predAddr = node.predecessor.Address
// 	}

// 	log.Printf("Node %s state - ID: %s, Address: %s, Successor: %s, Predecessor: %s",
// 		nodeID, node.ID.String(), node.Address, node.successor.Address, predAddr)
// }

// // Add this new test function after ExampleChordNetwork

// func TestDirectFileTransfer() {
// 	// Create two servers
// 	server1 := makeChordServer("127.0.0.1:3000")
// 	server2 := makeChordServer("127.0.0.1:4000")

// 	// Start servers
// 	go func() { log.Fatal(server1.Start()) }()
// 	time.Sleep(1 * time.Second)
// 	go func() { log.Fatal(server2.Start()) }()
// 	time.Sleep(1 * time.Second)

// 	// Establish direct connection
// 	server1.Transport.Dial("127.0.0.1:4000")
// 	time.Sleep(1 * time.Second)

// 	// Store a test file on server1
// 	fileContent := "This is a test file for direct transfer"
// 	cid, err := server1.store.WriteEncrypt(
// 		"test-direct", "test.txt", bytes.NewReader([]byte(fileContent)), 0, 0)
// 	if err != nil {
// 		log.Fatalf("Failed to store test file: %v", err)
// 	}
// 	log.Printf("Stored test file with CID: %d", cid)

// 	// Get direct peer connection
// 	server1.peerLock.Lock()
// 	var peer2 p2p.Peer
// 	for addr, peer := range server1.peers {
// 		if strings.Contains(addr, "4000") {
// 			peer2 = peer
// 			break
// 		}
// 	}
// 	server1.peerLock.Unlock()

// 	if peer2 == nil {
// 		log.Fatalf("Failed to find peer connection to server2")
// 	}

// 	// Send direct file transfer message
// 	msg := Message{
// 		Payload: MessageGetChordFile{
// 			CID:      string(rune(cid)),
// 			FromAddr: server1.Transport.Addr(),
// 		},
// 	}

// 	server2.handleMessageGetChordFile(server1.Transport.Addr(), msg.Payload.(MessageGetChordFile))

// 	log.Println("Direct file transfer test completed")
// }
