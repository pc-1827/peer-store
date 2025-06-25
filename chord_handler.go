package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/pc-1827/peer-store/p2p"
)

// Handle find successor request
// Fix the FindSuccessor handler to use the logical address from the message
func (s *FileServer) handleMessageFindSuccessor(from string, msg MessageFindSuccessor) error {
	if s.chordNode == nil {
		return fmt.Errorf("chord node not initialized")
	}

	// Get the ID from the message
	id := new(big.Int).SetBytes(msg.ID)

	// Find the successor
	successor, err := s.chordNode.FindSuccessor(id)
	if err != nil {
		return err
	}

	// Create the response message
	response := Message{
		Payload: MessageFindSuccessorResponse{
			NodeID:    successor.ID.Bytes(),
			NodeAddr:  successor.Address,
			RequestID: msg.RequestID,
		},
	}

	// The key fix: We need to use the FromAddr from the message to route back
	// to the logical node, not the connection address (from)
	logicalAddress := msg.FromAddr

	// Log both addresses for debugging
	log.Printf("[%s] FindSuccessor request from connection %s, logical node %s",
		s.Transport.Addr(), from, logicalAddress)

	// Try to find an existing peer for the logical address
	s.peerLock.Lock()
	var foundPeer p2p.Peer
	var foundAddr string

	// First try exact match
	if peer, ok := s.peers[logicalAddress]; ok {
		foundPeer = peer
		foundAddr = logicalAddress
	} else {
		// Look for any connection that might be to the same node
		for peerAddr, peer := range s.peers {
			if strings.HasPrefix(peerAddr, logicalAddress) ||
				strings.HasPrefix(logicalAddress, peerAddr) {
				foundPeer = peer
				foundAddr = peerAddr
				break
			}
		}
	}
	s.peerLock.Unlock()

	if foundPeer == nil {
		// If we couldn't find the logical address in our peer list,
		// we need to establish a direct connection
		log.Printf("[%s] No existing connection to logical address %s, dialing...",
			s.Transport.Addr(), logicalAddress)

		remoteAddr, err := s.Transport.Dial(logicalAddress)
		if err != nil {
			log.Printf("[%s] Failed to dial %s: %v",
				s.Transport.Addr(), logicalAddress, err)
			// Fall back to using the current connection
			foundPeer = s.peers[from]
			foundAddr = from
		} else {
			// Wait for connection to establish
			time.Sleep(200 * time.Millisecond)

			s.peerLock.Lock()
			foundPeer = s.peers[remoteAddr.String()]
			foundAddr = remoteAddr.String()
			s.peerLock.Unlock()
		}
	}

	if foundPeer == nil {
		return fmt.Errorf("couldn't find or establish connection to %s", logicalAddress)
	}

	log.Printf("[%s] Sending FindSuccessorResponse with RequestID: %s to %s",
		s.Transport.Addr(), msg.RequestID, foundAddr)

	// After finding the peer, add a fallback mechanism
	if foundPeer == nil {
		// Try a different approach - just use the current connection
		log.Printf("[%s] FALLBACK: Using the current connection to send response",
			s.Transport.Addr())

		foundPeer = s.peers[from]
		if foundPeer == nil {
			return fmt.Errorf("no connection available to respond")
		}
	}

	// Send the response and log the result in detail
	err = s.sendMessage(foundPeer, &response)
	if err != nil {
		log.Printf("[%s] ERROR sending FindSuccessorResponse: %v",
			s.Transport.Addr(), err)
		return err
	}

	log.Printf("[%s] Successfully sent FindSuccessorResponse for ID %s to %s",
		s.Transport.Addr(), msg.RequestID, foundPeer.RemoteAddr().String())

	return nil
}

// Handle find successor response
func (s *FileServer) handleMessageFindSuccessorResponse(from string, msg MessageFindSuccessorResponse) error {
	if s.chordNode == nil {
		return fmt.Errorf("chord node not initialized")
	}

	// Extract the original requester's address from the RequestID
	parts := strings.Split(msg.RequestID, "-")
	if len(parts) >= 2 {
		originalRequester := parts[0]
		log.Printf("[%s] Original requester for ID %s was %s (received from %s)",
			s.Transport.Addr(), msg.RequestID, originalRequester, from)

		// If we're not the original requester, forward the response
		if originalRequester != s.Transport.Addr() {
			log.Printf("[%s] This message needs to be forwarded to %s",
				s.Transport.Addr(), originalRequester)

			// Try all peers that might be the original requester
			s.peerLock.Lock()
			var peerFound bool
			for peerAddr, peer := range s.peers {
				if peerAddr == originalRequester || strings.HasPrefix(peerAddr, originalRequester) {
					log.Printf("[%s] Forwarding FindSuccessorResponse to original requester via %s",
						s.Transport.Addr(), peerAddr)

					// Forward asynchronously to avoid blocking
					go func(p p2p.Peer) {
						err := s.sendMessage(p, &Message{Payload: msg})
						if err != nil {
							log.Printf("[%s] Failed to forward response: %v",
								s.Transport.Addr(), err)
						}
					}(peer)

					peerFound = true
				}
			}
			s.peerLock.Unlock()

			if !peerFound {
				log.Printf("[%s] WARNING: Could not find peer for %s to forward response",
					s.Transport.Addr(), originalRequester)
			}

			// Continue processing in case we're also waiting for this response
		}
	}

	// Create a ChordNode from the response
	successor := &ChordNode{
		ID:         new(big.Int).SetBytes(msg.NodeID),
		Address:    msg.NodeAddr,
		fileServer: s,
	}

	log.Printf("[%s] Received FindSuccessorResponse with RequestID: %s from %s for node %s",
		s.Transport.Addr(), msg.RequestID, from, successor.Address)

	// Find and signal the waiting request
	value, ok := s.chordNode.pendingRequests.Load(msg.RequestID)
	if !ok {
		log.Printf("[%s] No waiting request found for ID: %s", s.Transport.Addr(), msg.RequestID)
		return nil
	}

	responseChan, ok := value.(chan *ChordNode)
	if !ok {
		log.Printf("[%s] Invalid channel type for request ID: %s", s.Transport.Addr(), msg.RequestID)
		return fmt.Errorf("invalid channel type")
	}

	// Send the response with a non-blocking channel write
	select {
	case responseChan <- successor:
		log.Printf("[%s] Sent successor response to waiting channel for request %s",
			s.Transport.Addr(), msg.RequestID)
	default:
		log.Printf("[%s] Could not send to response channel for request %s (might be closed)",
			s.Transport.Addr(), msg.RequestID)
	}

	return nil
}

// Handle get predecessor request
func (s *FileServer) handleMessageGetPredecessor(from string, msg MessageGetPredecessor) error {
	if s.chordNode == nil {
		return fmt.Errorf("chord node not initialized")
	}

	// Get the predecessor
	s.chordNode.mtx.RLock()
	pred := s.chordNode.predecessor
	s.chordNode.mtx.RUnlock()

	// Create response
	response := Message{
		Payload: MessageGetPredecessorResponse{
			HasPred: (pred != nil),
		},
	}

	// If we have a predecessor, include its details
	if pred != nil {
		response.Payload = MessageGetPredecessorResponse{
			NodeID:   pred.ID.Bytes(),
			NodeAddr: pred.Address,
			HasPred:  true,
		}
	}

	// Get the peer
	peer, ok := s.peers[from]
	if !ok {
		return fmt.Errorf("peer %s not found", from)
	}

	return s.sendMessage(peer, &response)
}

// Handle get predecessor response
func (s *FileServer) handleMessageGetPredecessorResponse(from string, msg MessageGetPredecessorResponse) error {
	if s.chordNode == nil {
		return fmt.Errorf("chord node not initialized")
	}

	if !msg.HasPred {
		// Node has no predecessor
		return nil
	}

	// Create a ChordNode from the response
	node := &ChordNode{
		ID:         new(big.Int).SetBytes(msg.NodeID),
		Address:    msg.NodeAddr,
		fileServer: s,
	}

	// Check if this node should be our successor
	s.chordNode.mtx.RLock()
	succAddr := s.chordNode.successor.Address
	s.chordNode.mtx.RUnlock()

	if from == succAddr && isBetween(node.ID, s.chordNode.ID, s.chordNode.successor.ID, false) {
		s.chordNode.mtx.Lock()
		s.chordNode.successor = node
		s.chordNode.mtx.Unlock()
	}

	return nil
}

// Handle notify message
func (s *FileServer) handleMessageNotify(from string, msg MessageNotify) error {
	if s.chordNode == nil {
		return fmt.Errorf("chord node not initialized")
	}

	// Create a ChordNode from the message
	node := &ChordNode{
		ID:         new(big.Int).SetBytes(msg.NodeID),
		Address:    msg.NodeAddr,
		fileServer: s,
	}

	// Notify the Chord node
	s.chordNode.notify(node)

	return nil
}

// Handle store chord file message
func (s *FileServer) handleMessageStoreChordFile(from string, msg MessageStoreChordFile) error {
	if s.chordNode == nil {
		return fmt.Errorf("chord node not initialized")
	}

	log.Printf("[%s] Received request to store file %s from %s",
		s.Transport.Addr(), msg.CID, from)

	// Get the peer
	peer, ok := s.peers[from]
	if !ok {
		return fmt.Errorf("peer %s not found", from)
	}

	// First, read the p2p.IncomingStream byte
	firstByte := make([]byte, 1)
	_, err := peer.Read(firstByte)
	if err != nil {
		return fmt.Errorf("failed to read stream signal: %w", err)
	}

	if firstByte[0] != p2p.IncomingStream {
		return fmt.Errorf("expected incoming stream signal, got %d", firstByte[0])
	}

	log.Printf("[%s] Received stream signal from %s", s.Transport.Addr(), from)

	// Read all data into a buffer
	fileBuffer := new(bytes.Buffer)
	bytesRead, err := io.Copy(fileBuffer, peer)
	if err != nil {
		return fmt.Errorf("failed to read file data: %w", err)
	}

	log.Printf("[%s] Read %d bytes from peer %s",
		s.Transport.Addr(), bytesRead, from)

	if bytesRead == 0 {
		return fmt.Errorf("received empty file data")
	}

	// Dump first few bytes for debugging
	if bytesRead > 0 {
		previewSize := 20
		if bytesRead < 20 {
			previewSize = int(bytesRead)
		}
		log.Printf("[%s] First %d bytes: %v",
			s.Transport.Addr(), previewSize, fileBuffer.Bytes()[:previewSize])
	}

	// Store locally
	size, err := s.store.WriteEncrypt(msg.CID, msg.Key, fileBuffer, 0, 0)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	log.Printf("[%s] Successfully stored %d bytes for file %s",
		s.Transport.Addr(), size, msg.CID)

	// Update responsibles
	s.chordNode.mtx.Lock()
	s.chordNode.responsibles[msg.CID] = true
	s.chordNode.mtx.Unlock()

	return nil
}

// Handle get chord file message
func (s *FileServer) handleMessageGetChordFile(from string, msg MessageGetChordFile) error {
	if s.chordNode == nil {
		return fmt.Errorf("chord node not initialized")
	}

	log.Printf("[%s] Received request for file %s from %s",
		s.Transport.Addr(), msg.CID, from)

	// Check if we have the file
	if !s.store.Has(msg.CID) {
		log.Printf("[%s] File %s not found locally", s.Transport.Addr(), msg.CID)
		return fmt.Errorf("file %s not found", msg.CID)
	}

	// Get the peer
	peer, ok := s.peers[from]
	if !ok {
		return fmt.Errorf("peer %s not found", from)
	}

	// Get the file data and metadata
	metadata, err := s.store.readMetadata(msg.CID)
	if err != nil {
		log.Printf("[%s] Failed to read metadata for %s: %v",
			s.Transport.Addr(), msg.CID, err)
		return fmt.Errorf("failed to read metadata: %w", err)
	}

	_, fileReader, err := s.store.Read(msg.CID)
	if err != nil {
		log.Printf("[%s] Failed to read file %s: %v",
			s.Transport.Addr(), msg.CID, err)
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Make sure to close the reader if it's a ReadCloser
	if rc, ok := fileReader.(io.ReadCloser); ok {
		defer rc.Close()
	}

	// Read all file data into a buffer
	fileData, err := io.ReadAll(fileReader)
	if err != nil {
		log.Printf("[%s] Failed to read file data for %s: %v",
			s.Transport.Addr(), msg.CID, err)
		return fmt.Errorf("failed to read file data: %w", err)
	}

	log.Printf("[%s] Retrieved file %s with name %s (%d bytes)",
		s.Transport.Addr(), msg.CID, metadata.FileName, len(fileData))

	// CRITICAL FIX: Use direct Write instead of Send to avoid protocol issues
	// First, send exactly one byte with value p2p.IncomingStream (2)
	log.Printf("[%s] Sending stream signal (value %d) to %s",
		s.Transport.Addr(), p2p.IncomingStream, from)

	_, err = peer.Write([]byte{p2p.IncomingStream})
	if err != nil {
		return fmt.Errorf("failed to send stream signal: %w", err)
	}

	// Send filename length (4 bytes)
	filenameLenBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(filenameLenBytes, uint32(len(metadata.FileName)))
	_, err = peer.Write(filenameLenBytes)
	if err != nil {
		return fmt.Errorf("failed to send filename length: %w", err)
	}

	// Send filename
	_, err = peer.Write([]byte(metadata.FileName))
	if err != nil {
		return fmt.Errorf("failed to send filename: %w", err)
	}

	// Send file data
	bytesWritten, err := peer.Write(fileData)
	if err != nil {
		return fmt.Errorf("failed to send file data: %w", err)
	}

	log.Printf("[%s] Successfully sent file %s (%d bytes) to %s",
		s.Transport.Addr(), msg.CID, bytesWritten, from)

	return nil
}

// Handle delete chord file message
func (s *FileServer) handleMessageDeleteChordFile(from string, msg MessageDeleteChordFile) error {
	if s.chordNode == nil {
		return fmt.Errorf("chord node not initialized")
	}

	// Remove from responsibles
	s.chordNode.mtx.Lock()
	delete(s.chordNode.responsibles, msg.CID)
	s.chordNode.mtx.Unlock()

	// Delete the file
	return s.Remove(msg.CID)
}
