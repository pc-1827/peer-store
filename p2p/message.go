package p2p

const (
	IncomingMessage = 0x1
	IncomingStream  = 0x2
)

// RPC holds any arbitary data that is sent across
// TCPtransport between two nodes in a network

type RPC struct {
	From    string
	Payload []byte
	Stream  bool
}
