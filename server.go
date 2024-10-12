package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pc-1827/distributed-file-system/crypto"
	"github.com/pc-1827/distributed-file-system/p2p"
)

type Network struct {
	EncType string
	EncKey  []byte
	Nonce   []byte
	Nodes   []string
}

var network Network

type FileServerOptions struct {
	ID                string
	StorageRoot       string
	PathTransformFunc PathTransformFunc
	Transport         p2p.Transport
}

type FileServer struct {
	FileServerOptions

	peerLock sync.Mutex
	peers    map[string]p2p.Peer

	store  *Store
	quitch chan struct{}
}

func NewFileServer(opts FileServerOptions) *FileServer {
	StoreOptions := StoreOptions{
		Root:              opts.StorageRoot,
		PathTransformFunc: opts.PathTransformFunc,
	}

	if len(opts.ID) == 0 {
		opts.ID = GenerateID()
	}
	return &FileServer{
		FileServerOptions: opts,
		store:             NewStore(StoreOptions),
		quitch:            make(chan struct{}),
		peers:             make(map[string]p2p.Peer),
	}
}

type Message struct {
	Payload any
}

func (s *FileServer) broadcast(msg *Message) error {
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(msg); err != nil {
		return err
	}

	for _, peer := range s.peers {
		peer.Send([]byte{p2p.IncomingMessage})
		if err := peer.Send(buf.Bytes()); err != nil {
			return err
		}
	}

	return nil
}

func (s *FileServer) sendMessage(peer p2p.Peer, msg *Message) error {
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(msg); err != nil {
		return err
	}

	peer.Send([]byte{p2p.IncomingMessage})
	if err := peer.Send(buf.Bytes()); err != nil {
		return err
	}

	return nil
}

type MessageGetFile struct {
	ID  string
	Key string
}

// Takes a key(file Name) and returns a Reader(data), checks if file is present
// locally using store.Has func, if yes then reads the files and returns a Reader.
// If not, then prepares a message and broadcasts to all peers reads the incoming
// binary data, decrypts and writes it to the local disk, and returns a reader to
// that received file.
func (s *FileServer) Get(key string) (io.Reader, string, error) {
	fmt.Printf("[%s] MSG.ID: (%s), MSG.Key: (%s)\n", s.Transport.Addr(), s.ID, key)
	if s.store.Has(s.ID, key) {
		fmt.Printf("[%s] serving file (%s) from the local disk\n", s.Transport.Addr(), key)
		_, r, f, err := s.store.ReadDecrypt(s.ID, key)
		return r, f, err
	}

	fmt.Printf("[%s] don't have the file (%s) locally, fetching from the network\n", s.Transport.Addr(), key)

	msg := Message{
		Payload: MessageGetFile{
			ID:  s.ID,
			Key: key,
		},
	}

	if err := s.broadcast(&msg); err != nil {
		return nil, "", err
	}

	time.Sleep(time.Millisecond * 500)

	for _, peer := range s.peers {
		var fileSize int64
		binary.Read(peer, binary.LittleEndian, &fileSize)
		n, err := s.store.Write(s.ID, key, io.LimitReader(peer, fileSize))
		if err != nil {
			return nil, "", err
		}

		fmt.Printf("[%s] received %d bytes over the network from (%s):\n ", s.Transport.Addr(), n, peer.RemoteAddr())

		peer.CloseStream()
	}

	_, r, f, err := s.store.ReadDecrypt(s.ID, key)
	return r, f, err
}

type MessageStoreFile struct {
	ID   string
	Key  string
	Size int64
}

// Takes a key(file name) and a Reader(data) writes the data to the disk,
// prepares and broadcasts a message to all peers to prepare them for receiving
// data, then writes encrypted data to the stream to all peers.

func (s *FileServer) Store(key string, r io.Reader) error {
	var (
		fileBuffer = new(bytes.Buffer)
		tee        = io.TeeReader(r, fileBuffer)
	)

	size, err := s.store.WriteEncrypt(s.ID, key, tee)
	if err != nil {
		return fmt.Errorf("failed to write encrypted data to local disk: %s", err)
	}

	msg := Message{
		Payload: MessageStoreFile{
			ID:   s.ID,
			Key:  key,
			Size: size,
		},
	}

	s.broadcast(&msg)

	time.Sleep(time.Millisecond * 5)

	peers := []io.Writer{}
	for _, peer := range s.peers {
		peers = append(peers, peer)
	}
	mw := io.MultiWriter(peers...)
	mw.Write([]byte{p2p.IncomingStream})
	n, err := encrypt(fileBuffer, mw)
	if err != nil {
		return err
	}

	fmt.Printf("[%s] received and written (%d) bytes to the disk\n", s.Transport.Addr(), n)
	return nil
}

type MessageDeleteFile struct {
	ID  string
	Key string
}

// Takes a key(file name) to be deleted in the peer network, prepares
// and broadcasts a message to all peers. Deletes the key on local disk
// using the store.Delete func.
func (s *FileServer) Remove(key string) error {
	msg := Message{
		Payload: MessageDeleteFile{
			ID:  s.ID,
			Key: key,
		},
	}

	if err := s.broadcast(&msg); err != nil {
		return err
	}

	if err := s.store.Delete(s.ID, key); err != nil {
		return fmt.Errorf("[%s] Failed to delete file (%s) from the disk: ", s.Transport.Addr(), key)
	}

	fmt.Printf("[%s] Deleting (%s) from the disk\n", s.Transport.Addr(), s.ID)

	time.Sleep(time.Second * 1)

	return nil
}

type MessageAddFile struct {
	Addr      string
	LocalAddr string
	Network   Network
}

func (s *FileServer) Add(addr string) error {
	var remoteAddr net.Addr
	var err error

	if len(network.Nodes) == 0 {
		network = Network{
			EncType: "CC20",
			EncKey:  crypto.NewEncryptionKey(),
			Nonce:   crypto.GenerateNonce(),
			Nodes:   append(network.Nodes, s.Transport.Addr()),
		}
	}
	network.Nodes = append(network.Nodes, addr)
	msg := Message{
		Payload: MessageAddFile{
			Addr:      "",
			LocalAddr: s.Transport.Addr(),
			Network:   network,
		},
	}

	s.broadcast(&msg)

	if len(addr) == 0 {
		return fmt.Errorf("address is empty")
	}

	fmt.Printf("[%s] Attempting to dial: (%s)\n", s.Transport.Addr(), addr)
	go func(addr string) {
		remoteAddr, err = s.Transport.Dial(addr)
		if err != nil {
			log.Printf("Failed to dial %s: %s\n", addr, err)
		}
	}(addr)

	time.Sleep(500 * time.Millisecond)

	// fmt.Printf("Peer Address: %s\n", peer.RemoteAddr().String())
	// fmt.Printf("Peer Map: %s\n", peersMap)

	peer, ok := s.peers[remoteAddr.String()]
	if !ok {
		return fmt.Errorf("peer not found")
	}

	msg = Message{
		Payload: MessageAddFile{
			Addr:      remoteAddr.String(),
			LocalAddr: s.Transport.Addr(),
			Network:   network,
		},
	}

	s.sendMessage(peer, &msg)
	return nil
}

func (s *FileServer) Start() error {
	if err := s.Transport.ListenAndAccept(); err != nil {
		return err
	}

	//s.bootstrapNetwork()

	s.loop()
	return nil
}

func (s *FileServer) Stop() {
	if s.quitch != nil {
		close(s.quitch)
	}
}

func (s *FileServer) OnPeer(p p2p.Peer) error {
	s.peerLock.Lock()
	defer s.peerLock.Unlock()

	s.peers[p.RemoteAddr().String()] = p

	log.Printf("[%s]: Peer connected: %s\n", p.LocalAddr().String(), p.RemoteAddr().String())
	return nil
}

func (s *FileServer) loop() {
	defer func() {
		log.Print("File server stopped due to error or quitch channel")
		s.Transport.Close()
	}()

	for {
		select {
		case rpc := <-s.Transport.Consume():
			var msg Message
			if err := gob.NewDecoder(bytes.NewReader(rpc.Payload)).Decode(&msg); err != nil {
				log.Printf("Failed to decode message: %s\n", err)
			}
			if err := s.handleMessage(rpc.From, &msg); err != nil {
				log.Println("handle message error:", err)
			}
		case <-s.quitch:
			return
		}
	}
}

func init() {
	gob.Register(MessageStoreFile{})
	gob.Register(MessageGetFile{})
	gob.Register(MessageDeleteFile{})
	gob.Register(MessageAddFile{})
}
