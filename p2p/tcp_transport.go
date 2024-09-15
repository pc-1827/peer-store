package p2p

import (
	"fmt"
	"net"
	"sync"
)

// TCPPeer represents the remote node in a TCP connection.
type TCPPeer struct {
	// conn is the underlying connection of the peer
	conn     net.Conn
	outbound bool
}

func NewTCPPeer(conn net.Conn, outbound bool) *TCPPeer {
	return &TCPPeer{
		conn:     conn,
		outbound: outbound,
	}
}

type TCPTransport struct {
	listenAddress string
	listener      net.Listener

	mu    sync.RWMutex
	peers map[net.Addr]Peer
}

func NewTCPTransport(listenAddress string) *TCPTransport {
	return &TCPTransport{
		listenAddress: listenAddress,
	}
}

func (t *TCPTransport) ListenAndAccept() error {
	var err error

	t.listener, err = net.Listen("tcp", t.listenAddress)
	if err != nil {
		return err
	}

	go t.startAcceptLoop()

	return nil
}

func (t *TCPTransport) startAcceptLoop() {
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			fmt.Printf("TCP accept error: %s\n", err)
		}
		go t.hanldeConn(conn)
	}
}

func (t *TCPTransport) hanldeConn(conn net.Conn) {
	peer := NewTCPPeer(conn, true)
	fmt.Printf("new incoming connection %v\n", peer)
}
