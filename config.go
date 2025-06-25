package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/pc-1827/peer-store/crypto"
)

// Config holds all command-line configuration for the peer storage system
type Config struct {
	Architecture string // "mesh" or "chord"
	Encryption   string // "AES", "CC20", "None"
	Split        bool   // Whether to split files across nodes (mesh only)
	JoinAddr     string // Address to join (empty for first node)
	ListenAddr   string // Address to listen on
	APIPort      int    // HTTP API port
}

// ParseConfig parses command-line flags into a Config struct
func ParseConfig() (*Config, error) {
	config := &Config{}

	flag.StringVar(&config.Architecture, "arch", "chord", "Architecture: mesh or chord")
	flag.StringVar(&config.Encryption, "enc", "AES", "Encryption type: AES, CC20, None")
	flag.BoolVar(&config.Split, "split", false, "Whether to split files across nodes (mesh only)")
	flag.StringVar(&config.JoinAddr, "joinAddr", "", "Address to join (empty for first node)")
	flag.StringVar(&config.ListenAddr, "addr", "127.0.0.1:3000", "Address to listen on")
	flag.IntVar(&config.APIPort, "apiport", 8080, "HTTP API port")

	flag.Parse()

	// Validate configuration
	if config.Architecture != "mesh" && config.Architecture != "chord" {
		return nil, fmt.Errorf("invalid architecture: %s (must be 'mesh' or 'chord')", config.Architecture)
	}

	// Validate encryption type
	if config.Encryption != "AES" && config.Encryption != "CC20" && config.Encryption != "None" {
		return nil, fmt.Errorf("unsupported encryption type: %s (must be 'AES', 'CC20', or 'None')", config.Encryption)
	}

	// Ensure split is only used with mesh architecture
	if config.Split && config.Architecture != "mesh" {
		return nil, fmt.Errorf("file splitting is only supported with mesh architecture")
	}

	// If this is not the first node, joinAddr must be provided
	if config.JoinAddr == "" && !isFirstNode(config.ListenAddr) {
		log.Printf("Warning: No join address provided. This will start a new network.")
	}

	return config, nil
}

// isFirstNode determines if this is likely to be the first node based on address
func isFirstNode(addr string) bool {
	// Simple heuristic - if it ends with port 3000, it's likely the first node
	return strings.HasSuffix(addr, ":3000")
}

// InitializeNetwork sets up the global network configuration
func InitializeNetwork(config *Config) {
	if config.Architecture == "mesh" {
		network = Network{
			EncType: config.Encryption,
			EncKey:  crypto.NewEncryptionKey(),
			Nonce:   crypto.GenerateNonce(),
			Nodes:   []string{config.ListenAddr},
			Split:   config.Split,
		}
	}
}
