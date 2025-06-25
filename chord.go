package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/pc-1827/peer-store/p2p"
)

const (
	M             = 160 // SHA-1 output size in bits
	StabilizeInt  = 500 * time.Millisecond
	FixFingersInt = 500 * time.Millisecond
	CheckPredInt  = 1 * time.Second
)

var (
	ErrNodeNotFound    = errors.New("node not found")
	ErrFileNotFound    = errors.New("file not found")
	ErrNodeUnreachable = errors.New("node unreachable")
)

// ChordNode represents a node in the Chord ring
type ChordNode struct {
	ID              *big.Int        // Node ID (hash of IP:port)
	Address         string          // Node address (IP:port)
	successor       *ChordNode      // Immediate successor in the ring
	predecessor     *ChordNode      // Immediate predecessor in the ring
	fingers         [M]*ChordNode   // Finger table for efficient lookups
	fileServer      *FileServer     // Reference to the underlying file server
	mtx             sync.RWMutex    // Mutex for thread safety
	stopChan        chan struct{}   // Channel to signal stopping
	responsibles    map[string]bool // Files this node is responsible for
	pendingRequests sync.Map        // map[string]chan *ChordNode
}

// NewChordNode creates a new Chord node
func NewChordNode(fileServer *FileServer) *ChordNode {
	addr := fileServer.Transport.Addr()
	node := &ChordNode{
		ID:           hashToNodeID(addr),
		Address:      addr,
		fileServer:   fileServer,
		stopChan:     make(chan struct{}),
		responsibles: make(map[string]bool),
	}

	// Initialize node as its own successor until a real successor is found
	node.successor = node

	return node
}

// InitChord initializes a Chord node
func (s *FileServer) InitChord() {
	s.chordNode = NewChordNode(s)
	s.chordNode.Create()
	log.Printf("[%s] Initialized Chord node", s.Transport.Addr())
}

// JoinChord makes this server join an existing Chord ring
func (s *FileServer) JoinChord(addr string) error {
	if s.chordNode == nil {
		s.chordNode = NewChordNode(s)
	}
	return s.chordNode.Join(addr)
}

// StoreChord stores a file using the Chord DHT
func (s *FileServer) StoreChord(key string, r io.Reader) (string, error) {
	if s.chordNode == nil {
		return "", fmt.Errorf("chord node not initialized")
	}
	return s.chordNode.StoreFile(key, r)
}

// GetChord retrieves a file using the Chord DHT
func (s *FileServer) GetChord(cid string) (io.Reader, string, error) {
	if s.chordNode == nil {
		return nil, "", fmt.Errorf("chord node not initialized")
	}
	return s.chordNode.GetFile(cid)
}

// DeleteChord deletes a file using the Chord DHT
func (s *FileServer) DeleteChord(cid string) error {
	if s.chordNode == nil {
		return fmt.Errorf("chord node not initialized")
	}
	return s.chordNode.DeleteFile(cid)
}

// Create initializes a new Chord ring with this node as the only member
func (n *ChordNode) Create() {
    n.mtx.Lock()
    n.predecessor = nil
    n.successor = n // Set self as successor initially
    // Initialize finger table to point to self
    for i := 0; i < M; i++ {
        n.fingers[i] = n
    }
    n.mtx.Unlock()

    // Start maintenance routines
    go n.stabilizeRoutine()
    go n.fixFingersRoutine()
    go n.checkPredecessorRoutine()

    log.Printf("[%s] Created a new Chord ring", n.Address)
}

// Join makes this node join an existing Chord ring through a known node
func (n *ChordNode) Join(addr string) error {
    // Don't try to join ourselves
    if addr == n.Address {
        n.Create()
        return nil
    }

    log.Printf("[%s] Attempting to join ring via %s", n.Address, addr)

    // Initialize our node for joining
    n.mtx.Lock()
    n.predecessor = nil
    n.mtx.Unlock()

    // Directly connect to the existing node
    remoteAddr, err := n.fileServer.Transport.Dial(addr)
    if err != nil {
        return fmt.Errorf("failed to dial %s: %w", addr, err)
    }

    // Wait for connection to establish
    time.Sleep(500 * time.Millisecond)

    // Use the remoteAddr as the node's successor
    n.mtx.Lock()
    n.successor = &ChordNode{
        ID:         hashToNodeID(remoteAddr.String()),
        Address:    remoteAddr.String(),
        fileServer: n.fileServer,
    }
    log.Printf("[%s] Setting initial successor to %s", n.Address, n.successor.Address)
    n.mtx.Unlock()

    // Immediately notify our successor about us
    n.sendNotify(n.successor.Address)

    // Start maintenance routines
    go n.stabilizeRoutine()
    go n.fixFingersRoutine()
    go n.checkPredecessorRoutine()

    log.Printf("[%s] Joined Chord ring via %s", n.Address, addr)
    return nil
}

// Stop gracefully shuts down the node
func (n *ChordNode) Stop() {
    close(n.stopChan)
}

// FindSuccessor finds the node responsible for the given ID
func (n *ChordNode) FindSuccessor(id *big.Int) (*ChordNode, error) {
    n.mtx.RLock()
    defer n.mtx.RUnlock()

    // Safety check for nil successor
    if n.successor == nil {
        log.Printf("[%s] ERROR: Nil successor detected in FindSuccessor, using self", n.Address)
        return n, nil
    }

    // Check if this is a single-node ring
    if n.successor.Address == n.Address {
        log.Printf("[%s] Single node ring, I'm responsible for everything", n.Address)
        return n, nil
    }

    // Check if the ID falls between us and our successor
    if isBetween(id, n.ID, n.successor.ID, true) {
        return n.successor, nil
    }

    // Otherwise, find the closest preceding node and ask it
    closestNode := n.closestPrecedingNode(id)
    log.Printf("[%s] Closest preceding node for %s is %s",
        n.Address, id.String(), closestNode.Address)

    if closestNode.Address == n.Address {
        // If the closest is ourself, return our successor
        log.Printf("[%s] I'm the closest preceding node, returning my successor %s",
            n.Address, n.successor.Address)
        return n.successor, nil
    }

    // Forward the request to the closest preceding node
    log.Printf("[%s] Forwarding FindSuccessor to %s", n.Address, closestNode.Address)
    return n.findSuccessorRemote(closestNode.Address, id)
}

// StoreFile stores a file in the DHT
func (n *ChordNode) StoreFile(key string, r io.Reader) (string, error) {
    // Read all data into memory first
    fileData, err := io.ReadAll(r)
    if err != nil {
        return "", fmt.Errorf("failed to read file data: %w", err)
    }

    log.Printf("[%s] Read %d bytes of file data for %s",
        n.Address, len(fileData), key)

    // Generate CID from the data
    cid := GenerateCID(bytes.NewReader(fileData))
    log.Printf("[%s] Generated CID: %s", n.Address, cid)

    // Hash the CID to get the Chord ID
    fileID := hashToNodeID(cid)
    log.Printf("[%s] Looking for node responsible for ID: %s",
        n.Address, fileID.String())

    // First try to find the successor locally to avoid network issues
    n.mtx.RLock()
    localSucc := n.successor
    myID := n.ID
    n.mtx.RUnlock()

    var responsible *ChordNode

    // Check if it's between us and our successor
    if isBetween(fileID, myID, localSucc.ID, true) {
        responsible = localSucc
        log.Printf("[%s] File %s is between me and my successor, responsible node is %s",
            n.Address, cid, responsible.Address)
    } else {
        // Otherwise use the full lookup
        log.Printf("[%s] File %s requires full lookup", n.Address, cid)
        var err error
        responsible, err = n.FindSuccessor(fileID)
        if err != nil {
            return "", fmt.Errorf("failed to find responsible node: %w", err)
        }
    }

    log.Printf("[%s] Responsible node for file %s is %s",
        n.Address, cid, responsible.Address)

    // If we are responsible, store locally
    if responsible.Address == n.Address {
        log.Printf("[%s] I'm responsible, storing file %s locally",
            n.Address, cid)

        // Use store directly
        size, err := n.fileServer.store.WriteEncrypt(
            cid, key, bytes.NewReader(fileData), 0, 0)

        if err != nil {
            return "", fmt.Errorf("failed to write file locally: %w", err)
        }

        log.Printf("[%s] Successfully stored %d bytes for file %s",
            n.Address, size, cid)

        // Add to responsibles
        n.mtx.Lock()
        n.responsibles[cid] = true
        n.mtx.Unlock()

        return cid, nil
    }

    // Otherwise, forward to the responsible node
    log.Printf("[%s] Forwarding file %s to responsible node %s",
        n.Address, cid, responsible.Address)

    // Try to get an existing connection or create a new one
    n.fileServer.peerLock.Lock()
    peer, ok := n.fileServer.peers[responsible.Address]
    n.fileServer.peerLock.Unlock()

    if !ok {
        // Try to dial
        log.Printf("[%s] No existing connection to %s, dialing...",
            n.Address, responsible.Address)

        remoteAddr, err := n.fileServer.Transport.Dial(responsible.Address)
        if err != nil {
            return "", fmt.Errorf("failed to dial responsible node: %w", err)
        }

        time.Sleep(200 * time.Millisecond)

        n.fileServer.peerLock.Lock()
        peer, ok = n.fileServer.peers[remoteAddr.String()]
        n.fileServer.peerLock.Unlock()

        if !ok {
            return "", fmt.Errorf("responsible node not in peer list after dialing")
        }
    }

    // Create store request message
    msg := Message{
        Payload: MessageStoreChordFile{
            CID:      cid,
            Key:      key,
            FromAddr: n.Address,
        },
    }

    // Send message
    err = n.fileServer.sendMessage(peer, &msg)
    if err != nil {
        return "", fmt.Errorf("failed to send store message: %w", err)
    }

    log.Printf("[%s] Sent store message to %s, now sending file data (%d bytes)",
        n.Address, responsible.Address, len(fileData))

    // Send file data with stream signal
    peer.Send([]byte{p2p.IncomingStream})
    bytesWritten, err := peer.Write(fileData)
    if err != nil {
        return "", fmt.Errorf("failed to send file data: %w", err)
    }

    log.Printf("[%s] Sent %d/%d bytes to %s for file %s",
        n.Address, bytesWritten, len(fileData), responsible.Address, cid)

    return cid, nil
}

// GetFile retrieves a file from the DHT
func (n *ChordNode) GetFile(cid string) (io.Reader, string, error) {
    log.Printf("[%s] GetFile: Looking for file %s", n.Address, cid)

    // If we have the file locally, return it
    if n.fileServer.store.Has(cid) {
        log.Printf("[%s] Found file %s locally", n.Address, cid)
        _, r, m, err := n.fileServer.store.ReadDecrypt(cid)
        return r, m.FileName, err
    }

    // Find the node responsible for this CID
    fileID := hashToNodeID(cid)
    responsible, err := n.FindSuccessor(fileID)
    if err != nil {
        return nil, "", fmt.Errorf("failed to find responsible node: %w", err)
    }

    log.Printf("[%s] Responsible node for file %s is %s",
        n.Address, cid, responsible.Address)

    // If we are responsible but don't have it, file doesn't exist
    if responsible.Address == n.Address {
        return nil, "", ErrFileNotFound
    }

    // Get the peer connection to the responsible node
    n.fileServer.peerLock.Lock()
    peer, ok := n.fileServer.peers[responsible.Address]
    n.fileServer.peerLock.Unlock()

    if !ok {
        // Try to dial the responsible node
        remoteAddr, err := n.fileServer.Transport.Dial(responsible.Address)
        if err != nil {
            return nil, "", fmt.Errorf("failed to dial responsible node: %w", err)
        }

        time.Sleep(100 * time.Millisecond) // Allow connection to establish

        n.fileServer.peerLock.Lock()
        peer, ok = n.fileServer.peers[remoteAddr.String()]
        n.fileServer.peerLock.Unlock()

        if !ok {
            return nil, "", fmt.Errorf("responsible node peer not found after dialing")
        }
    }

    // Create and send the request
    msg := Message{
        Payload: MessageGetChordFile{
            CID:      cid,
            FromAddr: n.Address,
        },
    }

    err = n.fileServer.sendMessage(peer, &msg)
    if err != nil {
        return nil, "", fmt.Errorf("failed to send request: %w", err)
    }

    // Wait for response using the same connection
    log.Printf("[%s] Waiting for file data from %s", n.Address, responsible.Address)

    // Read the stream signal - should be exactly one byte with value 2
    signalByte := make([]byte, 1)
    _, err = peer.Read(signalByte)
    if err != nil {
        return nil, "", fmt.Errorf("failed to read stream signal: %w", err)
    }

    // Check if we got the expected signal
    if signalByte[0] != p2p.IncomingStream {
        log.Printf("[%s] Warning: Expected stream signal %d, got %d",
            n.Address, p2p.IncomingStream, signalByte[0])
        // Continue anyway, but log the warning
    }

    // Read filename length (4 bytes)
    filenameLenBytes := make([]byte, 4)
    _, err = io.ReadFull(peer, filenameLenBytes)
    if err != nil {
        return nil, "", fmt.Errorf("failed to read filename length: %w", err)
    }

    filenameLen := binary.LittleEndian.Uint32(filenameLenBytes)
    if filenameLen > 1000 {
        return nil, "", fmt.Errorf("filename too long: %d", filenameLen)
    }

    // Read filename
    filenameBytes := make([]byte, filenameLen)
    _, err = io.ReadFull(peer, filenameBytes)
    if err != nil {
        return nil, "", fmt.Errorf("failed to read filename: %w", err)
    }
    filename := string(filenameBytes)

    // Now read file data (the rest of the stream)
    // Create a buffer to read all data
    fileDataBuffer := new(bytes.Buffer)
    tempBuf := make([]byte, 4096)

    // Read in chunks until we hit EOF or error
    for {
        n, err := peer.Read(tempBuf)
        if err != nil {
            if err == io.EOF {
                break
            }
            return nil, "", fmt.Errorf("error reading file data: %w", err)
        }

        if n == 0 {
            break
        }

        _, err = fileDataBuffer.Write(tempBuf[:n])
        if err != nil {
            return nil, "", fmt.Errorf("error writing to buffer: %w", err)
        }
    }

    fileData := fileDataBuffer.Bytes()
    log.Printf("[%s] Successfully received file '%s' (%d bytes)",
        n.Address, filename, len(fileData))

    return bytes.NewReader(fileData), filename, nil
}

// DeleteFile deletes a file from the DHT
func (n *ChordNode) DeleteFile(cid string) error {
    // Find the node responsible for this CID
    fileID := hashToNodeID(cid)
    responsible, err := n.FindSuccessor(fileID)
    if err != nil {
        return fmt.Errorf("failed to find responsible node: %w", err)
    }

    // If we are responsible, delete locally
    if responsible.Address == n.Address {
        log.Printf("[%s] Deleting file %s locally", n.Address, cid)

        // Remove from responsibles
        n.mtx.Lock()
        delete(n.responsibles, cid)
        n.mtx.Unlock()

        return n.fileServer.Remove(cid)
    }

    // Otherwise, forward to the responsible node
    log.Printf("[%s] Forwarding delete request for %s to responsible node %s",
        n.Address, cid, responsible.Address)

    // Get the peer
    n.fileServer.peerLock.Lock()
    peer, ok := n.fileServer.peers[responsible.Address]
    n.fileServer.peerLock.Unlock()

    if !ok {
        // Try to dial the responsible node
        remoteAddr, err := n.fileServer.Transport.Dial(responsible.Address)
        if err != nil {
            return fmt.Errorf("failed to dial responsible node: %w", err)
        }

        time.Sleep(100 * time.Millisecond) // Allow connection to establish

        n.fileServer.peerLock.Lock()
        peer, ok = n.fileServer.peers[remoteAddr.String()]
        n.fileServer.peerLock.Unlock()

        if !ok {
            return fmt.Errorf("responsible node peer not found after dialing")
        }
    }

    // Create delete request message
    msg := Message{
        Payload: MessageDeleteChordFile{
            CID:      cid,
            FromAddr: n.Address,
        },
    }

    // Send message to the responsible node
    return n.fileServer.sendMessage(peer, &msg)
}
