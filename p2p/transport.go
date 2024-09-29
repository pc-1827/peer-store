package p2p

import "net"

// Transport is anything that can send data across the nodes in
// a network, which could be {TCP, UDP, Websockets, etc.}
type Transport interface {
	Addr() string
	ListenAndAccept() error
	Consume() <-chan RPC
	Close() error
	Dial(string) error
}

// Peer is an interface that repersents a remote node in the network
type Peer interface {
	net.Conn
	Send([]byte) error
	CloseStream()
}
