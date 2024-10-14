package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
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
	Split   bool
}

var network Network

type FileServerOptions struct {
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

type MessageGetFile struct {
	CID string
}

// Takes a key(file Name) and returns a Reader(data), checks if file is present
// locally using store.Has func, if yes then reads the files and returns a Reader.
// If not, then prepares a message and broadcasts to all peers reads the incoming
// binary data, decrypts and writes it to the local disk, and returns a reader to
// that received file.
func (s *FileServer) Get(cid string) (io.Reader, string, error) {
	if !(network.Split) {
		return s.getAll(cid)
	} else {
		return s.getSplit(cid)
	}
}

func (s *FileServer) getAll(cid string) (io.Reader, string, error) {
	fmt.Printf("[%s] MSG.CID: (%s)\n", s.Transport.Addr(), cid)
	if s.store.Has(cid) {
		fmt.Printf("[%s] serving file (%s) from the local disk\n", s.Transport.Addr(), cid)
		_, r, f, err := s.store.ReadDecrypt(cid)
		return r, f, err
	}

	fmt.Printf("[%s] don't have the file (%s) locally, fetching from the network\n", s.Transport.Addr(), cid)

	msg := Message{
		Payload: MessageGetFile{
			CID: cid,
		},
	}

	if err := s.broadcast(&msg); err != nil {
		return nil, "", err
	}

	time.Sleep(time.Millisecond * 500)

	for _, peer := range s.peers {
		var fileSize int64
		if err := binary.Read(peer, binary.LittleEndian, &fileSize); err != nil {
			return nil, "", fmt.Errorf("failed to read file size: %s", err)
		}
		fmt.Printf("File Size: %d\n", fileSize)
		n, err := s.store.writeStream(cid, io.LimitReader(peer, fileSize))
		if err != nil {
			return nil, "", err
		}

		fmt.Printf("[%s] received %d bytes over the network from (%s):\n", s.Transport.Addr(), n, peer.RemoteAddr())

		var keySize int64
		if err := binary.Read(peer, binary.LittleEndian, &keySize); err != nil {
			return nil, "", fmt.Errorf("failed to read key size: %s", err)
		}
		fmt.Printf("Key size: %d\n", keySize/256)
		data, err := io.ReadAll(io.LimitReader(peer, int64(keySize/256)+1))
		if err != nil {
			return nil, "", fmt.Errorf("failed to read key data: %s", err)
		}
		fmt.Printf("Key data: %s\n", string(data))
		fileName := strings.ReplaceAll(string(data), "\x00", "")
		s.store.writeMetadata(cid, fileName, 0, 0)

		fmt.Printf("[%s] received %d bytes of key over the network from (%s):\n", s.Transport.Addr(), len(data)-1, peer.RemoteAddr())

		peer.CloseStream()
	}

	_, r, f, err := s.store.ReadDecrypt(cid)
	return r, f, err
}

func (s *FileServer) getSplit(cid string) (io.Reader, string, error) {
	reader := bytes.NewReader([]byte(cid))
	return reader, "", nil
}

type MessageStoreFile struct {
	CID        string
	Key        string
	Part       int
	TotalParts int
	Size       int64
}

// Takes a key(file name) and a Reader(data) writes the data to the disk,
// prepares and broadcasts a message to all peers to prepare them for receiving
// data, then writes encrypted data to the stream to all peers.

func (s *FileServer) Store(key string, r io.Reader) (string, error) {
	if !(network.Split) {
		return s.storeAll(key, r)
	} else {
		return s.storeSplit(key, r)
	}
}

func (s *FileServer) storeAll(key string, r io.Reader) (string, error) {
	var (
		buffer     = new(bytes.Buffer)
		fileBuffer = new(bytes.Buffer)
		tee        = io.TeeReader(r, buffer)
		reader     = io.TeeReader(buffer, fileBuffer)
	)
	cid := GenerateCID(tee)
	size, err := s.store.WriteEncrypt(cid, key, reader, 0, 0)
	if err != nil {
		return "", fmt.Errorf("failed to write encrypted data to local disk: %s", err)
	}

	msg := Message{
		Payload: MessageStoreFile{
			CID:        cid,
			Key:        key,
			Part:       0,
			TotalParts: 0,
			Size:       size,
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
		return "", err
	}

	fmt.Printf("[%s] received and written (%d) bytes to the disk\n", s.Transport.Addr(), n)
	return cid, nil
}

func (s *FileServer) storeSplit(key string, r io.Reader) (string, error) {
	var (
		buffer = new(bytes.Buffer)
		reader = io.TeeReader(r, buffer)
	)

	cid := GenerateCID(reader)
	parts, err := s.splitFile(buffer)
	if err != nil {
		return "", fmt.Errorf("unable to split the file: %s", err)
	}

	totalParts := len(parts)

	fileBuffer := new(bytes.Buffer)
	tee := io.TeeReader(parts[0], fileBuffer)
	size, err := s.store.WriteEncrypt(cid, key, tee, 1, totalParts)
	if err != nil {
		return "", fmt.Errorf("failed to write encrypted data to local disk: %s", err)
	}

	fmt.Printf("[%s] written part %d (%d bytes) to disk\n", s.Transport.Addr(), 1, size)

	parts = parts[1:]
	partIndex := 2
	for peerAddr, peer := range s.peers {
		if len(parts) == 0 {
			break
		}

		part := parts[0]
		partBuffer := new(bytes.Buffer)
		partSize, _ := io.Copy(partBuffer, part)
		parts = parts[1:]

		msg := Message{
			Payload: MessageStoreFile{
				CID:        cid,
				Key:        key,
				Part:       partIndex,
				TotalParts: totalParts,
				Size:       int64(partSize),
			},
		}

		if err := s.sendMessage(peer, &msg); err != nil {
			return "", fmt.Errorf("failed to send message to peer %s: %s", peerAddr, err)
		}

		time.Sleep(5 * time.Millisecond)

		peer.Send([]byte{p2p.IncomingStream})
		n, err := io.Copy(peer, partBuffer)
		if err != nil {
			return "", fmt.Errorf("failed to send part to peer %s: %s", peerAddr, err)
		}

		fmt.Printf("[%s] sent part %d (%d bytes) to peer %s\n", s.Transport.Addr(), partIndex, n, peer.RemoteAddr())
		partIndex++
	}

	return cid, nil
}

type MessageDeleteFile struct {
	CID string
}

// Takes a key(file name) to be deleted in the peer network, prepares
// and broadcasts a message to all peers. Deletes the key on local disk
// using the store.Delete func.
func (s *FileServer) Remove(cid string) error {
	msg := Message{
		Payload: MessageDeleteFile{
			CID: cid,
		},
	}

	if err := s.broadcast(&msg); err != nil {
		return err
	}

	if err := s.store.Delete(cid); err != nil {
		return fmt.Errorf("[%s] Failed to delete file (%s) from the disk: ", s.Transport.Addr(), cid)
	}

	fmt.Printf("[%s] Deleting (%s) from the disk\n", s.Transport.Addr(), cid)

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
			Split:   true,
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
