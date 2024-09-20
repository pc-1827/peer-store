package main

import (
	"fmt"
	"log"

	"github.com/pc-1827/distributed-file-system/p2p"
)

type FileServerOptions struct {
	StorageRoot       string
	PathTransformFunc PathTransformFunc
	Transport         p2p.Transport
}

type FileServer struct {
	FileServerOptions

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
	}
}

func (s *FileServer) Start() error {
	if err := s.Transport.ListenAndAccept(); err != nil {
		return err
	}

	s.loop()
	return nil
}

func (s *FileServer) Stop() {
	if s.quitch != nil {
		close(s.quitch)
	}
}

func (s *FileServer) loop() {
	defer func() {
		log.Print("File server stopped due to quitch channel")
		s.Transport.Close()
	}()

	for {
		select {
		case msg := <-s.Transport.Consume():
			fmt.Printf("Received message from %v: %v\n", msg.From, string(msg.Payload))
		case <-s.quitch:
			return
		}
	}
}
