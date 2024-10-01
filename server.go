package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/pc-1827/distributed-file-system/p2p"
)

type FileServerOptions struct {
	ID                string
	EncKey            []byte
	StorageRoot       string
	PathTransformFunc PathTransformFunc
	Transport         p2p.Transport
	BootstrapNodes    []string
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
		opts.ID = generateID()
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

type MessageGetFile struct {
	ID  string
	Key string
}

func (s *FileServer) Get(key string) (io.Reader, error) {
	if s.store.Has(s.ID, key) {
		fmt.Printf("[%s] serving file (%s) from the local disk\n", s.Transport.Addr(), key)
		_, r, err := s.store.Read(s.ID, key)
		return r, err
	}

	fmt.Printf("[%s] don't have the file (%s) locally, fetching from the network\n", s.Transport.Addr(), key)

	msg := Message{
		Payload: MessageGetFile{
			ID:  s.ID,
			Key: hashKey(key),
		},
	}

	if err := s.broadcast(&msg); err != nil {
		return nil, err
	}

	time.Sleep(time.Millisecond * 500)

	for _, peer := range s.peers {
		var fileSize int64
		binary.Read(peer, binary.LittleEndian, &fileSize)
		n, err := s.store.WriteDecrypt(s.EncKey, s.ID, key, io.LimitReader(peer, fileSize))
		// n, err := s.store.Write(key, io.LimitReader(peer, fileSize))
		if err != nil {
			return nil, err
		}
		// fileBuffer := new(bytes.Buffer)
		// n, err := io.CopyN(fileBuffer, peer, 22)
		// if err != nil {
		// 	return nil, err
		// }

		fmt.Printf("[%s] received %d bytes over the network from (%s):\n ", s.Transport.Addr(), n, peer.RemoteAddr())

		peer.CloseStream()
	}

	_, r, err := s.store.Read(s.ID, key)
	return r, err
}

type MessageStoreFile struct {
	ID   string
	Key  string
	Size int64
}

func (s *FileServer) Store(key string, r io.Reader) error {
	var (
		fileBuffer = new(bytes.Buffer)
		tee        = io.TeeReader(r, fileBuffer)
	)

	size, err := s.store.Write(s.ID, key, tee)
	if err != nil {
		return err
	}

	msg := Message{
		Payload: MessageStoreFile{
			ID:   s.ID,
			Key:  hashKey(key),
			Size: size + 16,
		},
	}

	if err := s.broadcast(&msg); err != nil {
		return err
	}

	time.Sleep(time.Millisecond * 5)

	peers := []io.Writer{}
	for _, peer := range s.peers {
		peers = append(peers, peer)
	}
	mw := io.MultiWriter(peers...)
	mw.Write([]byte{p2p.IncomingStream})
	n, err := copyEncrypt(s.EncKey, fileBuffer, mw)
	if err != nil {
		return err
	}

	fmt.Printf("[%s] received and written (%d) bytes to the disk\n", s.Transport.Addr(), n)
	return nil
	// buf := new(bytes.Buffer)
	// tee := io.TeeReader(r, buf)

	// if err := s.store.Write(key, tee); err != nil {
	// 	return err
	// }

	// p := &DataMessage{
	// 	Key:  key,
	// 	Data: buf.Bytes(),
	// }

	// //fmt.Println(buf.Bytes())

	// return s.broadcast(&Message{
	// 	From:    "todo",
	// 	Payload: p,
	// })
}

type MessageDeleteFile struct {
	ID  string
	Key string
}

func (s *FileServer) Remove(key string) error {
	msg := Message{
		Payload: MessageDeleteFile{
			ID:  s.ID,
			Key: hashKey(key),
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

func (s *FileServer) Start() error {
	if err := s.Transport.ListenAndAccept(); err != nil {
		return err
	}

	s.bootstrapNetwork()

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

	log.Printf("Peer connected: %s\n", p.RemoteAddr().String())
	return nil
}

func (s *FileServer) handleMessage(from string, msg *Message) error {
	switch v := msg.Payload.(type) {
	case MessageStoreFile:
		return s.handleMessageStoreFile(from, v)
	case MessageGetFile:
		return s.handleMessageGetFile(from, v)
	case MessageDeleteFile:
		return s.handleMessageDeleteFile(v)
	}

	return nil
}

func (s *FileServer) handleMessageGetFile(from string, msg MessageGetFile) error {
	if !s.store.Has(msg.ID, msg.Key) {
		return fmt.Errorf("[%s] file (%s) is not present in the disk", s.Transport.Addr(), msg.Key)
	}

	fmt.Printf("[%s] got file (%s) from the disk, serving over the network\n", s.Transport.Addr(), msg.Key)
	fileSize, r, err := s.store.Read(msg.ID, msg.Key)
	if err != nil {
		return err
	}

	if rc, ok := r.(io.ReadCloser); ok {
		fmt.Println("closing the read closer")
		defer rc.Close()
	}

	peer, ok := s.peers[from]
	if !ok {
		return fmt.Errorf("peer (%s) is not in peer map", from)
	}

	peer.Send([]byte{p2p.IncomingStream})
	binary.Write(peer, binary.LittleEndian, fileSize)

	n, err := io.Copy(peer, r)
	if err != nil {
		return err
	}

	fmt.Printf("[%s] written %d bytes over the network to %s\n", s.Transport.Addr(), n, from)
	return nil
}

func (s *FileServer) handleMessageStoreFile(from string, msg MessageStoreFile) error {
	//fmt.Printf("recv store file msg %v: %+v\n", from, msg)
	peer, ok := s.peers[from]
	if !ok {
		return fmt.Errorf("peer (%s) could not be found in the peer list", from)
	}

	n, err := s.store.Write(msg.ID, msg.Key, io.LimitReader(peer, msg.Size))
	if err != nil {
		return err
	}

	fmt.Printf("[%s] written %d bytes to the disk\n", s.Transport.Addr(), n)

	//peer.(*p2p.TCPPeer).Wg.Done()
	peer.CloseStream()

	return nil
}

func (s *FileServer) handleMessageDeleteFile(msg MessageDeleteFile) error {
	if err := s.store.Delete(msg.ID, msg.Key); err != nil {
		return fmt.Errorf("[%s] Failed to delete file (%s) from the disk: ", s.Transport.Addr(), msg.Key)
	}
	fmt.Printf("[%s] Deleting from the disk\n", s.Transport.Addr())
	return nil
}

func (s *FileServer) bootstrapNetwork() error {
	for _, addr := range s.BootstrapNodes {
		if len(addr) == 0 {
			continue
		}
		fmt.Printf("[%s] Attempting to dial: (%s)\n", s.Transport.Addr(), addr)
		go func(addr string) {
			if err := s.Transport.Dial(addr); err != nil {
				log.Printf("Failed to dial %s: %s\n", addr, err)
			}
		}(addr)
	}
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
			// fmt.Printf("payload: %+v\n", msg.Payload)
			// //fmt.Printf("Received: %s\n", string(msg.Payload.([]byte)))

			// peer, ok := s.peers[rpc.From]
			// if !ok {
			// 	panic("peer not found in peer map")
			// }

			// b := make([]byte, 1000)
			// if _, err := peer.Read(b); err != nil {
			// 	panic(err)
			// }
			// // if err := s.handleMessage(&m); err != nil {
			// // 	log.Println(err)

			// // }
			// fmt.Printf("%s\n", string(b))

			// peer.(*p2p.TCPPeer).Wg.Done()
		case <-s.quitch:
			return
		}
	}
}

func init() {
	gob.Register(MessageStoreFile{})
	gob.Register(MessageGetFile{})
	gob.Register(MessageDeleteFile{})
}
