package p2p

import (
	"errors"
	"fmt"
	"log"
	"net"
)

// TCPPeer represents the remote node in a TCP connection.
type TCPPeer struct {
	// conn is the underlying connection of the peer
	conn     net.Conn
	outbound bool
}

// Close closes the underlying connection of the peer.
func (p *TCPPeer) Close() error {
	return p.conn.Close()
}

func NewTCPPeer(conn net.Conn, outbound bool) *TCPPeer {
	return &TCPPeer{
		conn:     conn,
		outbound: outbound,
	}
}

type TCPTransportOptions struct {
	ListenAddress string
	HandShakeFunc HandShakeFunc
	Decoder       Decoder
	OnPeer        func(Peer) error
}

type TCPTransport struct {
	TCPTransportOptions
	listener net.Listener
	rpcch    chan RPC
}

func NewTCPTransport(opts TCPTransportOptions) *TCPTransport {
	return &TCPTransport{
		TCPTransportOptions: opts,
		rpcch:               make(chan RPC),
	}
}

// Consume returns a channel that can be used to receive RPC messages
// from another peer in the network.
func (t *TCPTransport) Consume() <-chan RPC {
	return t.rpcch
}

func (t *TCPTransport) Close() error {
	return t.listener.Close()
}

func (t *TCPTransport) ListenAndAccept() error {
	var err error

	t.listener, err = net.Listen("tcp", t.ListenAddress)
	if err != nil {
		return err
	}

	go t.startAcceptLoop()

	log.Printf("Listening on %s\n", t.ListenAddress)

	return nil
}

func (t *TCPTransport) startAcceptLoop() {
	for {
		conn, err := t.listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			fmt.Printf("TCP accept error: %s\n", err)
		}
		fmt.Printf("new incoming connection %v\n", conn)

		go t.handleConn(conn)
	}
}

func (t *TCPTransport) handleConn(conn net.Conn) {
	var err error
	defer func() {
		fmt.Printf("Dropping Peer connection: %s\n", err)
		conn.Close()
	}()

	peer := NewTCPPeer(conn, true)

	err = t.HandShakeFunc(peer)
	if err != nil {
		conn.Close()
		fmt.Printf("TCP handshake error: %s\n", err)
		return
	}

	if t.OnPeer != nil {
		err = t.OnPeer(peer)
		if err != nil {
			return
		}
	}

	// Read loop
	rpc := RPC{}
	for {
		err = t.Decoder.Decode(conn, &rpc)
		if err != nil {
			fmt.Printf("TCP decode error: %s\n", err)
			continue
		}

		rpc.From = conn.RemoteAddr()
		t.rpcch <- rpc
	}
}
