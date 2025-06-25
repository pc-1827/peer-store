package main

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/pc-1827/peer-store/p2p"
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
	Config            *Config // Add config field
}

type FileServer struct {
	FileServerOptions

	peerLock sync.Mutex
	peers    map[string]p2p.Peer

	store     *Store
	quitch    chan struct{}
	chordNode *ChordNode
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
	case MessageFindSuccessor:
		return s.handleMessageFindSuccessor(from, v)
	case MessageFindSuccessorResponse:
		return s.handleMessageFindSuccessorResponse(from, v)
	case MessageGetPredecessor:
		return s.handleMessageGetPredecessor(from, v)
	case MessageGetPredecessorResponse:
		return s.handleMessageGetPredecessorResponse(from, v)
	case MessageNotify:
		return s.handleMessageNotify(from, v)
	case MessageStoreChordFile:
		return s.handleMessageStoreChordFile(from, v)
	case MessageGetChordFile:
		return s.handleMessageGetChordFile(from, v)
	case MessageDeleteChordFile:
		return s.handleMessageDeleteChordFile(from, v)
	}

	return nil
}

// Start the file server and API server
func (s *FileServer) Start() error {
	// Start the transport
	if err := s.Transport.ListenAndAccept(); err != nil {
		return err
	}

	// Start API server
	if s.Config != nil {
		s.startAPIServer()
	}

	// Start the main server loop
	s.loop()
	return nil
}

// startAPIServer starts the HTTP API server
func (s *FileServer) startAPIServer() {
	apiAddr := fmt.Sprintf("127.0.0.1:%d", s.Config.APIPort)

	// Register API endpoints
	http.HandleFunc("/api/store", s.handleAPIStore)
	http.HandleFunc("/api/retrieve", s.handleAPIRetrieve)
	http.HandleFunc("/api/delete", s.handleAPIDelete)
	http.HandleFunc("/api/status", s.handleAPIStatus)

	go func() {
		log.Printf("Starting API server on %s", apiAddr)
		if err := http.ListenAndServe(apiAddr, nil); err != nil {
			log.Printf("API server error: %v", err)
		}
	}()
}

// API Handlers

// handleAPIStore processes file upload requests
func (s *FileServer) handleAPIStore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 10MB)
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Error parsing form: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Get file from form
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error getting file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename := header.Filename
	log.Printf("API: Received request to store file: %s", filename)

	// Store file based on architecture
	var cid string
	if s.Config.Architecture == "chord" {
		cid, err = s.StoreChord(filename, file)
	} else {
		cid, err = s.Store(filename, file)
	}

	if err != nil {
		log.Printf("API: Error storing file: %v", err)
		http.Error(w, "Error storing file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success response
	response := map[string]string{
		"cid":      cid,
		"filename": filename,
		"status":   "success",
	}

	log.Printf("API: Successfully stored file with CID: %s", cid)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleAPIRetrieve processes file download requests
func (s *FileServer) handleAPIRetrieve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get CID from query parameter
	cid := r.URL.Query().Get("cid")
	if cid == "" {
		http.Error(w, "Missing CID parameter", http.StatusBadRequest)
		return
	}

	log.Printf("API: Received request to retrieve file with CID: %s", cid)

	// Retrieve file based on architecture
	var reader io.Reader
	var filename string
	var err error

	if s.Config.Architecture == "chord" {
		reader, filename, err = s.GetChord(cid)
	} else {
		reader, filename, err = s.Get(cid)
	}

	if err != nil {
		log.Printf("API: Error retrieving file: %v", err)
		http.Error(w, "Error retrieving file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Set content disposition header for downloading
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(filename)))
	w.Header().Set("Content-Type", "application/octet-stream")

	// Stream file to response
	bytesCopied, err := io.Copy(w, reader)
	if err != nil {
		log.Printf("API: Error streaming file: %v", err)
		return
	}

	log.Printf("API: Successfully served file %s (%d bytes)", filename, bytesCopied)
}

// handleAPIDelete processes file deletion requests
func (s *FileServer) handleAPIDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get CID from query parameter or form
	var cid string
	if r.Method == http.MethodDelete {
		cid = r.URL.Query().Get("cid")
	} else {
		r.ParseForm()
		cid = r.FormValue("cid")
	}

	if cid == "" {
		http.Error(w, "Missing CID parameter", http.StatusBadRequest)
		return
	}

	log.Printf("API: Received request to delete file with CID: %s", cid)

	// Delete file based on architecture
	var err error
	if s.Config.Architecture == "chord" {
		err = s.DeleteChord(cid)
	} else {
		err = s.Remove(cid)
	}

	if err != nil {
		log.Printf("API: Error deleting file: %v", err)
		http.Error(w, "Error deleting file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success response
	response := map[string]string{
		"cid":    cid,
		"status": "deleted",
	}

	log.Printf("API: Successfully deleted file with CID: %s", cid)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleAPIStatus returns the status of the node
func (s *FileServer) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Collect node information
	var nodeInfo map[string]interface{}

	if s.Config.Architecture == "chord" {
		// Get Chord-specific status
		predAddr := "nil"
		if s.chordNode.predecessor != nil {
			predAddr = s.chordNode.predecessor.Address
		}

		nodeInfo = map[string]interface{}{
			"architecture": "chord",
			"address":      s.chordNode.Address,
			"id":           s.chordNode.ID.String(),
			"successor":    s.chordNode.successor.Address,
			"predecessor":  predAddr,
			"fileCount":    len(s.chordNode.responsibles),
		}
	} else {
		// Get mesh-specific status
		nodeInfo = map[string]interface{}{
			"architecture": "mesh",
			"address":      s.Transport.Addr(),
			"peers":        len(s.peers),
			"network":      network,
		}
	}

	// Add common information
	nodeInfo["split"] = network.Split
	nodeInfo["encryption"] = network.EncType

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodeInfo)
}

// Add this function to server.go (after the existing imports and type definitions)

// registerGobTypes registers all types used in gob encoding/decoding for message passing
func registerGobTypes() {
	gob.Register(MessageStoreFile{})
	gob.Register(MessageGetFile{})
	gob.Register(MessageDeleteFile{})
	gob.Register(MessageAddFile{})
	gob.Register(MessageFindSuccessor{})
	gob.Register(MessageFindSuccessorResponse{})
	gob.Register(MessageGetPredecessor{})
	gob.Register(MessageGetPredecessorResponse{})
	gob.Register(MessageNotify{})
	gob.Register(MessageStoreChordFile{})
	gob.Register(MessageGetChordFile{})
	gob.Register(MessageDeleteChordFile{})
}
