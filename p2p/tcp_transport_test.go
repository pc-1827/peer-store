package p2p

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTcpTransport(t *testing.T) {
	listenAddress := ":4000"
	tcpTransport := NewTCPTransport(listenAddress)

	assert.Equal(t, listenAddress, tcpTransport.listenAddress)

	assert.Nil(t, tcpTransport.ListenAndAccept())
}
