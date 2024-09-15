package p2p

import "net"

// RPC holds any arbitary data that is sent across
// TCPtransport between two nodes in a network

type RPC struct {
	From    net.Addr
	Payload []byte
}
