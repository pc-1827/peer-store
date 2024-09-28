package p2p

// RPC holds any arbitary data that is sent across
// TCPtransport between two nodes in a network

type RPC struct {
	From    string
	Payload []byte
}
