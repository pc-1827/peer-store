package main

import (
	"crypto/sha1"
	"fmt"
	"io"
	"log"
	"math/big"
	"math/rand"
	"strings"
	"time"

	"github.com/pc-1827/peer-store/p2p"
)

// closestPrecedingNode finds the closest finger preceding the given ID
func (n *ChordNode) closestPrecedingNode(id *big.Int) *ChordNode {
	for i := M - 1; i >= 0; i-- {
		if n.fingers[i] != nil &&
			isBetween(n.fingers[i].ID, n.ID, id, false) {
			return n.fingers[i]
		}
	}
	return n
}

// findSuccessorRemote asks a remote node to find the successor for an ID
func (n *ChordNode) findSuccessorRemote(addr string, id *big.Int) (*ChordNode, error) {
	// Create a unique request ID with source and destination
	requestID := fmt.Sprintf("%s-%s-%d", n.Address, addr, time.Now().UnixNano())
	log.Printf("[%s] Creating FindSuccessor request with ID: %s for node ID: %s",
		n.Address, requestID, id.String())

	// Create response channel with buffer to prevent deadlocks
	responseChan := make(chan *ChordNode, 1)

	// Register the response channel
	n.pendingRequests.Store(requestID, responseChan)
	defer n.pendingRequests.Delete(requestID)

	// Find all peers to the target node
	n.fileServer.peerLock.Lock()
	var peerToUse p2p.Peer
	var directPeer bool

	// First try exact address match
	if peer, ok := n.fileServer.peers[addr]; ok {
		peerToUse = peer
		directPeer = true
		log.Printf("[%s] Found direct peer connection to %s", n.Address, addr)
	} else {
		// Look for any peer that might be the same node but different connection
		for peerAddr, peer := range n.fileServer.peers {
			if strings.HasPrefix(peerAddr, addr) || strings.HasPrefix(addr, peerAddr) {
				peerToUse = peer
				log.Printf("[%s] Using similar peer %s for target %s", n.Address, peerAddr, addr)
				break
			}
		}
	}
	n.fileServer.peerLock.Unlock()

	// If no suitable peer found, try to dial
	if peerToUse == nil {
		log.Printf("[%s] No existing connection to %s, dialing...", n.Address, addr)
		remoteAddr, err := n.fileServer.Transport.Dial(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to dial %s: %w", addr, err)
		}

		// Wait for connection to establish
		time.Sleep(300 * time.Millisecond)

		n.fileServer.peerLock.Lock()
		newPeer, ok := n.fileServer.peers[remoteAddr.String()]
		n.fileServer.peerLock.Unlock()

		if !ok {
			return nil, fmt.Errorf("peer %s not found after dialing", remoteAddr.String())
		}

		peerToUse = newPeer
		log.Printf("[%s] Established new connection to %s via %s",
			n.Address, addr, remoteAddr.String())
	}

	// Send the message
	msg := Message{
		Payload: MessageFindSuccessor{
			ID:        id.Bytes(),
			FromAddr:  n.Address, // This is critical - the logical address
			RequestID: requestID,
		},
	}

	log.Printf("[%s] Sending FindSuccessor request to %s via %s",
		n.Address, addr, peerToUse.RemoteAddr().String())
	err := n.fileServer.sendMessage(peerToUse, &msg)
	if err != nil {
		return nil, fmt.Errorf("failed to send FindSuccessor message: %w", err)
	}

	// Create timeout timer with a longer duration for the first request
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()

	// Wait for response or timeout
	select {
	case successor := <-responseChan:
		log.Printf("[%s] Received FindSuccessor response for request %s: %s",
			n.Address, requestID, successor.Address)
		return successor, nil
	case <-timer.C:
		// Before giving up, check if we need to retry on a different connection
		if directPeer {
			log.Printf("[%s] Timeout on direct connection, will retry", n.Address)
			return n.findSuccessorRemote(addr, id)
		}
		log.Printf("[%s] Timeout waiting for FindSuccessor response to request %s",
			n.Address, requestID)
		return nil, fmt.Errorf("timeout waiting for FindSuccessor response")
	}
}

// Stabilize verifies and updates the successor
func (n *ChordNode) stabilize() {
	n.mtx.RLock()
	succ := n.successor
	n.mtx.RUnlock()

	// If we are our own successor, no need to stabilize
	if succ.Address == n.Address {
		return
	}

	// Ask successor for its predecessor
	predMsg := Message{
		Payload: MessageGetPredecessor{
			FromAddr: n.Address,
		},
	}

	// Get the peer
	n.fileServer.peerLock.Lock()
	succPeer, ok := n.fileServer.peers[succ.Address]
	n.fileServer.peerLock.Unlock()

	if !ok {
		log.Printf("[%s] Can't stabilize - successor %s not in peer list",
			n.Address, succ.Address)
		return
	}

	// Send message to get predecessor
	err := n.fileServer.sendMessage(succPeer, &predMsg)
	if err != nil {
		log.Printf("[%s] Failed to get predecessor from %s: %v",
			n.Address, succ.Address, err)
		return
	}

	// The response will come through the normal message handling
	// and will call n.handleGetPredecessorResponse

	// Send notification to successor about ourselves
	notifyMsg := Message{
		Payload: MessageNotify{
			NodeID:   n.ID.Bytes(),
			NodeAddr: n.Address,
		},
	}

	err = n.fileServer.sendMessage(succPeer, &notifyMsg)
	if err != nil {
		log.Printf("[%s] Failed to notify successor %s: %v",
			n.Address, succ.Address, err)
	}
}

// Notify is called when a node thinks it might be our predecessor
func (n *ChordNode) notify(node *ChordNode) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	// Update predecessor if:
	// 1. We don't have one, or
	// 2. The node is in (predecessor, us)
	if n.predecessor == nil ||
		isBetween(node.ID, n.predecessor.ID, n.ID, false) {
		log.Printf("[%s] Setting predecessor to %s", n.Address, node.Address)
		oldPred := n.predecessor
		n.predecessor = node

		// Log the change
		if oldPred == nil {
			log.Printf("[%s] Changed predecessor from nil to %s", n.Address, node.Address)
		} else {
			log.Printf("[%s] Changed predecessor from %s to %s",
				n.Address, oldPred.Address, node.Address)
		}

		// Update our successor if we're the only node in the ring
		if n.successor.Address == n.Address && n.predecessor != nil {
			// This node was previously alone in the ring, now update its successor
			log.Printf("[%s] Updating successor from self to %s", n.Address, n.predecessor.Address)
			n.successor = n.predecessor
		}

		// If predecessor changed, check if we need to transfer files
		if oldPred == nil || oldPred.ID.Cmp(node.ID) != 0 {
			go n.transferResponsibilities()
		}
	}
}

// Add this function to improve the stabilize mechanism
func (n *ChordNode) sendNotify(targetAddr string) {
	// Get the peer
	n.fileServer.peerLock.Lock()
	peer, ok := n.fileServer.peers[targetAddr]
	n.fileServer.peerLock.Unlock()

	if !ok {
		log.Printf("[%s] Can't notify %s - not in peer list", n.Address, targetAddr)
		return
	}

	// Send notification
	notifyMsg := Message{
		Payload: MessageNotify{
			NodeID:   n.ID.Bytes(),
			NodeAddr: n.Address,
		},
	}

	err := n.fileServer.sendMessage(peer, &notifyMsg)
	if err != nil {
		log.Printf("[%s] Failed to send notification to %s: %v",
			n.Address, targetAddr, err)
	} else {
		log.Printf("[%s] Sent notification to %s", n.Address, targetAddr)
	}
}

// fixFinger updates a random finger table entry
func (n *ChordNode) fixFinger() {
	// Choose a random finger to fix
	i := randInt(0, M)

	// Calculate the ID for this finger
	fingerID := fingerID(n.ID, i)

	// Find the successor for this ID
	successor, err := n.FindSuccessor(fingerID)
	if err != nil {
		log.Printf("[%s] Failed to fix finger %d: %v", n.Address, i, err)
		return
	}

	n.mtx.Lock()
	n.fingers[i] = successor
	n.mtx.Unlock()
}

// checkPredecessor checks if the predecessor is still alive
func (n *ChordNode) checkPredecessor() {
	n.mtx.RLock()
	pred := n.predecessor
	n.mtx.RUnlock()

	if pred == nil {
		return
	}

	// Check if predecessor is still reachable
	n.fileServer.peerLock.Lock()
	_, ok := n.fileServer.peers[pred.Address]
	n.fileServer.peerLock.Unlock()

	if !ok {
		// Try to dial the predecessor
		_, err := n.fileServer.Transport.Dial(pred.Address)
		if err != nil {
			// Predecessor is down, remove it
			n.mtx.Lock()
			n.predecessor = nil
			n.mtx.Unlock()
			log.Printf("[%s] Predecessor %s is down, removed",
				n.Address, pred.Address)
		}
	}
}

// Background maintenance routines
func (n *ChordNode) stabilizeRoutine() {
	ticker := time.NewTicker(StabilizeInt)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n.stabilize()
		case <-n.stopChan:
			return
		}
	}
}

func (n *ChordNode) fixFingersRoutine() {
	ticker := time.NewTicker(FixFingersInt)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n.fixFinger()
		case <-n.stopChan:
			return
		}
	}
}

func (n *ChordNode) checkPredecessorRoutine() {
	ticker := time.NewTicker(CheckPredInt)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n.checkPredecessor()
		case <-n.stopChan:
			return
		}
	}
}

// transferResponsibilities transfers files to the new predecessor if needed
func (n *ChordNode) transferResponsibilities() {
	n.mtx.RLock()
	pred := n.predecessor
	filesToTransfer := make([]string, 0)

	// Identify files that should be transferred
	for cid := range n.responsibles {
		fileID := hashToNodeID(cid)
		if pred != nil && isBetween(fileID, pred.ID, n.ID, true) {
			// This file now belongs to our predecessor
			filesToTransfer = append(filesToTransfer, cid)
		}
	}
	n.mtx.RUnlock()

	// Transfer each file
	for _, cid := range filesToTransfer {
		// Get the file data
		reader, fileName, err := n.fileServer.getAll(cid)
		if err != nil {
			log.Printf("[%s] Failed to read file %s for transfer: %v",
				n.Address, cid, err)
			continue
		}

		// Get the peer
		n.fileServer.peerLock.Lock()
		peer, ok := n.fileServer.peers[pred.Address]
		n.fileServer.peerLock.Unlock()

		if !ok {
			log.Printf("[%s] Predecessor %s not in peer list", n.Address, pred.Address)
			continue
		}

		// Send transfer message
		msg := Message{
			Payload: MessageStoreChordFile{
				CID:      cid,
				Key:      fileName,
				FromAddr: n.Address,
				Transfer: true,
			},
		}

		err = n.fileServer.sendMessage(peer, &msg)
		if err != nil {
			log.Printf("[%s] Failed to send transfer message: %v", n.Address, err)
			continue
		}

		// Send the file data
		peer.Send([]byte{p2p.IncomingStream})
		_, err = io.Copy(peer, reader)
		if err != nil {
			log.Printf("[%s] Failed to transfer file data: %v", n.Address, err)
			continue
		}

		// Remove from our responsibles
		n.mtx.Lock()
		delete(n.responsibles, cid)
		n.mtx.Unlock()

		log.Printf("[%s] Transferred file %s to predecessor %s",
			n.Address, cid, pred.Address)
	}
}

// Helper functions
func hashToNodeID(str string) *big.Int {
	h := sha1.Sum([]byte(str))
	return new(big.Int).SetBytes(h[:])
}

func fingerID(id *big.Int, i int) *big.Int {
	// Start = (n + 2^(i-1)) mod 2^m
	m := big.NewInt(int64(M))
	two := big.NewInt(2)

	// 2^(i-1)
	power := new(big.Int).Exp(two, big.NewInt(int64(i)), nil)

	// n + 2^(i-1)
	sum := new(big.Int).Add(id, power)

	// 2^m
	mod := new(big.Int).Exp(two, m, nil)

	// (n + 2^(i-1)) mod 2^m
	return new(big.Int).Mod(sum, mod)
}

func isBetween(id, from, to *big.Int, inclusive bool) bool {
	// Handle equality immediately
	if from.Cmp(to) == 0 {
		// If from equals to, then it's a single-node ring
		return inclusive // Only true if inclusive
	}

	if from.Cmp(to) < 0 {
		// Normal case: from < to (no wrap-around)
		if inclusive {
			result := (id.Cmp(from) >= 0 && id.Cmp(to) <= 0)
			return result
		} else {
			result := (id.Cmp(from) > 0 && id.Cmp(to) < 0)
			return result
		}
	} else {
		// Wrap-around case: from > to
		if inclusive {
			result := (id.Cmp(from) >= 0 || id.Cmp(to) <= 0)
			return result
		} else {
			result := (id.Cmp(from) > 0 || id.Cmp(to) < 0)
			return result
		}
	}
}

func randInt(min, max int) int {
	return min + rand.Intn(max-min)
}
