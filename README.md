# Peer-Store: Distributed P2P Storage System

Peer-Store is a flexible, distributed, decentralized peer-to-peer storage system that allows you to instantly set up a network of storage nodes. It provides multiple architectures, encryption options, and a simple HTTP API for interacting with the distributed storage network.

## Features

### Network Architectures

- **Chord DHT**: A distributed hash table architecture based on the Chord protocol, providing O(log n) lookups and automatic load balancing
- **Mesh Network**: A fully connected mesh network where each node communicates directly with every other node

### Storage Features

- **Content Addressable Storage (CAS)**: Files are addressed by their content hash, ensuring data integrity
- **Multiple Encryption Options**:
  - AES encryption
  - ChaCha20 encryption
  - Unencrypted storage (None)
- **File Splitting** (Mesh architecture only): Distributes file parts across multiple nodes for redundancy
- **Automatic Node Discovery**: Nodes can automatically join existing networks

### Interface

- **HTTP API**: RESTful API for storing, retrieving, and deleting files
- **Command-line Configuration**: Flexible configuration through command-line flags
- **Node Status Monitoring**: Check the status of any node in the network

## Upcoming Features

- Redundancy options for extra copies
- Additional encryption options and key management
- Enhanced error recovery and network resilience
- Access control and authentication mechanisms
- Bandwidth controls and throttling options
- Advanced node health monitoring and statistics
- Garbage collection for routine cleanup

## Getting Started

### Prerequisites

- Go 1.19 or higher

### Building the Project

```bash
git clone https://github.com/yourusername/peer-store.git
cd peer-store
go build -o peer-store *.go
```

### Running a Node

To start a node with default settings:

```bash
./peer-store
```

#### Configuration Options

- `-arch`: Network architecture: mesh or chord (default: chord)
- `-enc`: Encryption type: AES, CC20, None (default: AES)
- `-split`: Whether to split files across nodes, mesh architecture only (default: false)
- `-joinAddr`: Address to join, empty for first node (default: "")
- `-addr`: Address to listen on (default: "127.0.0.1:3000")
- `-apiport`: HTTP API port (default: 8080)

### Usage Examples

#### Storing a File

```bash
# Using curl
curl -X POST -F "file=@/path/to/your/file.txt" http://localhost:8080/store

# Response
{"hash":"bafc12a5c4c66c9b2a5d9d45a27dbf33e8dc4c4b","nodes":["192.168.1.10:8080","192.168.1.11:8080"]}
```

#### Retrieving a File

```bash
# Using curl
curl -X GET http://localhost:8080/retrieve/bafc12a5c4c66c9b2a5d9d45a27dbf33e8dc4c4b -o retrieved_file.txt
```

#### Deleting a File

```bash
# Using curl
curl -X DELETE http://localhost:8080/delete/bafc12a5c4c66c9b2a5d9d45a27dbf33e8dc4c4b
```

#### Checking Node Status

```bash
curl -X GET http://localhost:8080/status
```

## Setting Up a Network

### Chord DHT Network Example

1. Start the first node:
   ```bash
   ./peer-store -arch chord -enc AES -addr 127.0.0.1:3000 -apiport 8080
   ```

2. Join additional nodes to the network:
   ```bash
   ./peer-store -arch chord -enc AES -addr 127.0.0.1:3001 -apiport 8081 -joinAddr 127.0.0.1:3000
   ./peer-store -arch chord -enc AES -addr 127.0.0.1:3002 -apiport 8082 -joinAddr 127.0.0.1:3000
   ```

### Mesh Network with File Splitting Example

1. Start the first node:
   ```bash
   ./peer-store -arch mesh -enc CC20 -addr 127.0.0.1:3000 -apiport 8080 -split
   ```

2. Join additional nodes to the network:
   ```bash
   ./peer-store -arch mesh -enc CC20 -addr 127.0.0.1:3001 -apiport 8081 -split -joinAddr 127.0.0.1:3000
   ./peer-store -arch mesh -enc CC20 -addr 127.0.0.1:3002 -apiport 8082 -split -joinAddr 127.0.0.1:3000
   ```
