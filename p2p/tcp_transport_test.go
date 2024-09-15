package p2p

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTcpTransport(t *testing.T) {
	tcpOpts := TCPTransportOptions{
		ListenAddress: ":3000",
		HandShakeFunc: NOTHandShakeFunc,
		Decoder:       DefaultDecoder{},
	}
	tcpTransport := NewTCPTransport(tcpOpts)

	assert.Equal(t, tcpTransport.ListenAddress, ":3000")

	assert.Nil(t, tcpTransport.ListenAndAccept())
}
