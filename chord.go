package main

import (
	"crypto/sha1"
	"math/big"
	"sync"
	"time"
)

const M = 160

type Node struct {
	ID          *big.Int
	Address     string
	successor   *Node
	predecessor *Node
	fingers     [M]*Node
	mutex       sync.RWMutex
	stopCh      chan struct{}
}

func NewNode(address string) *Node {
	return &Node{
		ID:      hashToBigInt(address),
		Address: address,
		stopCh:  make(chan struct{}),
	}
}

func (n *Node) Create() {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.successor = n
	n.predecessor = nil
	for i := 0; i < M; i++ {
		n.fingers[i] = n
	}
}

func (n *Node) Join(knownNode *Node) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.predecessor = nil
	n.successor = knownNode.findSuccessor(n.ID)
}

func (n *Node) findSuccessor(id *big.Int) *Node {
	current := n
	if n == n.successor {
		return n
	}
	if betweenEq(id, current.ID, current.successor.ID) {
		return current.successor
	}
	closest := current.closestPrecedingNode(id)
	if closest == current {
		return current.successor.findSuccessor(id)
	}
	return closest.findSuccessor(id)
}

func (n *Node) closestPrecedingNode(id *big.Int) *Node {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	for i := M - 1; i >= 0; i-- {
		f := n.fingers[i]
		if f != nil && between(f.ID, n.ID, id) {
			return f
		}
	}
	return n
}

func (n *Node) Stabilize() {
	for {
		select {
		case <-n.stopCh:
			return
		case <-time.After(time.Second * 3):
			n.mutex.Lock()
			succ := n.successor
			if succ == nil {
				n.mutex.Unlock()
				continue
			}
			x := succ.predecessor
			if x != nil && between(x.ID, n.ID, succ.ID) {
				n.successor = x
			}
			n.successor.notify(n)
			n.mutex.Unlock()
		}
	}
}

func (n *Node) notify(candidate *Node) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	if n.predecessor == nil || between(candidate.ID, n.predecessor.ID, n.ID) {
		n.predecessor = candidate
	}
}

func (n *Node) FixFingers() {
	nextIndex := 0
	for {
		select {
		case <-n.stopCh:
			return
		case <-time.After(time.Second * 3):
			n.mutex.Lock()
			next := nextIndex
			start := new(big.Int).Add(n.ID, twoToPower(next))
			start.Mod(start, twoToPower(M))
			n.fingers[next] = n.findSuccessor(start)
			nextIndex = (nextIndex + 1) % M
			n.mutex.Unlock()
		}
	}
}

func (n *Node) AddPeer(newAddr string, knownNode *Node) *Node {
	newNode := NewNode(newAddr)
	newNode.Join(knownNode)
	return newNode
}

func (n *Node) Lookup(key string) *Node {
	keyID := hashToBigInt(key)
	return n.findSuccessor(keyID)
}

func (n *Node) Stop() {
	close(n.stopCh)
}

func hashToBigInt(str string) *big.Int {
	h := sha1.Sum([]byte(str))
	return new(big.Int).SetBytes(h[:])
}

func twoToPower(exp int) *big.Int {
	e := big.NewInt(1)
	return e.Lsh(e, uint(exp))
}

func between(x, a, b *big.Int) bool {
	if a.Cmp(b) < 0 {
		return (x.Cmp(a) > 0) && (x.Cmp(b) < 0)
	}
	return (x.Cmp(a) > 0) || (x.Cmp(b) < 0)
}

func betweenEq(x, a, b *big.Int) bool {
	if a.Cmp(b) < 0 {
		return (x.Cmp(a) > 0 && x.Cmp(b) <= 0) || x.Cmp(a) == 0 || x.Cmp(b) == 0
	}
	return (x.Cmp(a) > 0 || x.Cmp(b) < 0) || x.Cmp(a) == 0 || x.Cmp(b) == 0
}
