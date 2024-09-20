package p2p

// Transport is anything that can send data across the nodes in
// a network, which could be {TCP, UDP, Websockets, etc.}
type Transport interface {
	ListenAndAccept() error
	Consume() <-chan RPC
	Close() error
}

// Peer is an interface that repersents a remote node in the network
type Peer interface {
	Close() error
}
