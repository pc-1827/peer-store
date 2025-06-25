package main

// Message wrapper for all message types
type Message struct {
	Payload any
}

// MessageFindSuccessor is used to find the successor of an ID
type MessageFindSuccessor struct {
	ID        []byte // The ID to find the successor for
	FromAddr  string // Address of the requesting node
	RequestID string
}

// MessageFindSuccessorResponse is the response to a FindSuccessor request
type MessageFindSuccessorResponse struct {
	NodeID    []byte // ID of the successor node
	NodeAddr  string // Address of the successor node
	RequestID string
}

// MessageGetPredecessor asks a node for its predecessor
type MessageGetPredecessor struct {
	FromAddr string // Address of the requesting node
}

// MessageGetPredecessorResponse is the response to a GetPredecessor request
type MessageGetPredecessorResponse struct {
	NodeID   []byte // ID of the predecessor node (if any)
	NodeAddr string // Address of the predecessor node (if any)
	HasPred  bool   // Whether the node has a predecessor
}

// MessageNotify notifies a node about a potential predecessor
type MessageNotify struct {
	NodeID   []byte // ID of the potential predecessor
	NodeAddr string // Address of the potential predecessor
}

// MessageStoreChordFile stores a file in the DHT
type MessageStoreChordFile struct {
	CID      string // Content ID of the file
	Key      string // Original filename
	FromAddr string // Address of the requesting node
	Transfer bool   // Whether this is a transfer due to node join/leave
}

// MessageGetChordFile retrieves a file from the DHT
type MessageGetChordFile struct {
	CID      string // Content ID of the file
	FromAddr string // Address of the requesting node
}

// MessageDeleteChordFile deletes a file from the DHT
type MessageDeleteChordFile struct {
	CID      string // Content ID of the file
	FromAddr string // Address of the requesting node
}

// MessageGetFile retrieves a file
type MessageGetFile struct {
	CID string
}

// MessageStoreFile stores a file
type MessageStoreFile struct {
	CID        string
	Key        string
	Part       int
	TotalParts int
	Size       int64
}

// MessageDeleteFile deletes a file
type MessageDeleteFile struct {
	CID string
}

// MessageAddFile adds a file
type MessageAddFile struct {
	Addr      string
	LocalAddr string
	Network   Network
}
