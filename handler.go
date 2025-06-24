package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/pc-1827/peer-store/p2p"
)

func (s *FileServer) handleMessage(from string, msg *Message) error {
	switch v := msg.Payload.(type) {
	case MessageStoreFile:
		return s.handleMessageStoreFile(from, v)
	case MessageGetFile:
		return s.handleMessageGetFile(from, v)
	case MessageDeleteFile:
		return s.handleMessageDeleteFile(v)
	case MessageAddFile:
		return s.handleMessageAddFile(from, v)
	}

	return nil
}

type Response struct {
	Metadata Metadata
	FileSize int64
}

// Checks if the requested file is present or not. If present reads the file,
// and writes the encrypted binary data to the peer message was sent from.

func (s *FileServer) handleMessageGetFile(from string, msg MessageGetFile) error {
	fmt.Printf("[%s] MSG.CID: (%s)\n", s.Transport.Addr(), msg.CID)

	if !s.store.Has(msg.CID) {
		return fmt.Errorf("[%s] file (%s) is not present in the disk", s.Transport.Addr(), msg.CID)
	}

	metadata, err := s.store.readMetadata(msg.CID)
	if err != nil {
		return fmt.Errorf("error reading metadata: %s", err)
	}

	fmt.Printf("[%s] got file part (%d) from the disk, serving over the network (%s)\n", s.Transport.Addr(), metadata.Part, msg.CID)
	fileSize, r, err := s.store.Read(msg.CID)
	if err != nil {
		return err
	}

	if rc, ok := r.(io.ReadCloser); ok {
		defer rc.Close()
	}

	peer, ok := s.peers[from]
	if !ok {
		return fmt.Errorf("peer (%s) is not in peer map", from)
	}
	if !(network.Split) {
		keySize := int64(len(metadata.FileName))

		peer.Send([]byte{p2p.IncomingStream})
		binary.Write(peer, binary.LittleEndian, keySize)
		binary.Write(peer, binary.LittleEndian, fileSize)

		n, err := io.Copy(peer, bytes.NewReader([]byte(metadata.FileName)))
		if err != nil {
			return err
		}

		fmt.Printf("[%s] written %d bytes of key over the network to %s\n", s.Transport.Addr(), n, from)

		time.Sleep(5 * time.Millisecond)
		n, err = io.Copy(peer, r)
		if err != nil {
			return err
		}

		fmt.Printf("[%s] sent file %d bytes over the network to %s\n", s.Transport.Addr(), n, from)

	} else {
		partNumber := int64(metadata.Part)

		peer.Send([]byte{p2p.IncomingStream})
		binary.Write(peer, binary.LittleEndian, partNumber) 
		binary.Write(peer, binary.LittleEndian, fileSize)  

		n, err := io.Copy(peer, r)
		if err != nil {
			return err
		}

		fmt.Printf("[%s] sent file part %d (%d bytes) over the network to %s\n", s.Transport.Addr(), metadata.Part, n, from)
	}

	return nil
}

// Gets the peer in the received message and writes the data received to
// the local disk.
func (s *FileServer) handleMessageStoreFile(from string, msg MessageStoreFile) error {
	peer, ok := s.peers[from]
	if !ok {
		return fmt.Errorf("peer (%s) could not be found in the peer list", from)
	}

	fmt.Println(msg)

	dataBuffer := new(bytes.Buffer)
	if _, err := io.Copy(dataBuffer, io.LimitReader(peer, msg.Size)); err != nil {
		return fmt.Errorf("failed to read data: %s", err)
	}

	n, err := s.store.WriteEncrypt(msg.CID, msg.Key, dataBuffer, msg.Part, msg.TotalParts)
	if err != nil {
		return err
	}

	fmt.Printf("[%s] written part (%d) %d bytes to the disk\n", s.Transport.Addr(), msg.Part, n)

	peer.CloseStream()

	return nil
}

// Finds the file to delete from the received message and then calls the
// store.Delete function to remove from the local disk
func (s *FileServer) handleMessageDeleteFile(msg MessageDeleteFile) error {
	if err := s.store.Delete(msg.CID); err != nil {
		return fmt.Errorf("[%s] Failed to delete file (%s) from the disk: ", s.Transport.Addr(), msg.CID)
	}
	fmt.Printf("[%s] Deleting from the disk\n", s.Transport.Addr())
	return nil
}

func (s *FileServer) handleMessageAddFile(from string, msg MessageAddFile) error {
	if len(msg.Addr) == 0 {
		network.Nodes = msg.Network.Nodes
		return nil
	}

	network = msg.Network

	peer, ok := s.peers[from]
	if !ok {
		return fmt.Errorf("peer not found")
	}

	fmt.Printf("Peer: %s\n", peer.LocalAddr().String())

	peersMap := msg.Network.Nodes
	fmt.Printf("PeerMapStr: %s\n", peersMap)
	// Dial every peer in the map except the sender and the new peer
	for _, addr := range peersMap {
		if addr == msg.LocalAddr || addr == msg.Addr {
			continue
		}
		if len(addr) == 0 {
			continue
		}
		fmt.Printf("Address to be dialed %s\n", addr)
		go func(addr string) {
			if _, err := s.Transport.Dial(addr); err != nil {
				log.Printf("Failed to dial %s: %s\n", addr, err)
			}
		}(addr)
	}
	return nil
}
