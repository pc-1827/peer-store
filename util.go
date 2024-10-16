package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"log"

	"github.com/pc-1827/distributed-file-system/p2p"
)

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

func (s *FileServer) splitFile(r io.Reader) ([]io.Reader, error) {
	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, r); err != nil {
		return nil, fmt.Errorf("failed to read data from reader: %s", err)
	}
	data := buffer.Bytes()
	fileSize := len(data)

	fmt.Println(fileSize)

	numPeers := len(network.Nodes)
	if numPeers == 0 {
		return nil, fmt.Errorf("no peers available to split the file")
	}

	remainder := fileSize % numPeers
	partSize := fileSize / numPeers
	lengthArray := make([]int, numPeers)
	for i := range lengthArray {
		if remainder > 0 {
			lengthArray[i] = partSize + 1
			remainder--
		} else {
			lengthArray[i] = partSize
		}
	}

	readers := make([]io.Reader, numPeers)
	offset := 0
	for i, length := range lengthArray {
		readers[i] = bytes.NewReader(data[offset : offset+length])
		offset += length
	}

	return readers, nil
}

func (s *FileServer) combineFile(readerMap map[int]io.Reader) (io.Reader, error) {
	combinedBuffer := new(bytes.Buffer)

	for i := 1; i <= len(readerMap); i++ {
		reader, ok := readerMap[i]
		if !ok {
			return nil, fmt.Errorf("missing part %d", i)
		}

		if _, err := io.Copy(combinedBuffer, reader); err != nil {
			return nil, fmt.Errorf("failed to combine part %d: %s", i, err)
		}
	}

	return combinedBuffer, nil
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
