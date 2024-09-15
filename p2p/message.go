package p2p

import "net"

// Message holds any arbitary data that is sent across
// TCPtransport between two nodes in a network

type Message struct {
	From    net.Addr
	Payload []byte
}
