package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pc-1827/peer-store/crypto"
	"github.com/pc-1827/peer-store/p2p"
)

// Constants for testing
const (
	TestStorageRoot = "test_storage"
	TestOutputDir   = "test_output"
)

// setupTestNetwork creates a network of file servers for testing
func setupTestNetwork(t *testing.T, encType string) ([]*FileServer, func()) {
	// Create test directories
	os.RemoveAll(TestStorageRoot)
	os.RemoveAll(TestOutputDir)

	if err := os.MkdirAll(TestOutputDir, 0755); err != nil {
		t.Fatalf("Failed to create test output directory: %v", err)
	}

	// Create servers
	servers := make([]*FileServer, 4)
	ports := []string{"3000", "4000", "5000", "6000"}

	// Initialize servers
	for i := 0; i < 4; i++ {
		addr := fmt.Sprintf("127.0.0.1:%s", ports[i])
		root := filepath.Join(TestStorageRoot, fmt.Sprintf("server%d", i+1))

		tcpOptions := p2p.TCPTransportOptions{
			ListenAddress: addr,
			HandShakeFunc: p2p.NOTHandShakeFunc,
			Decoder:       p2p.DefaultDecoder{},
		}

		transport := p2p.NewTCPTransport(tcpOptions)

		serverOpts := FileServerOptions{
			StorageRoot:       root,
			PathTransformFunc: CASPathTransformFunc,
			Transport:         transport,
		}

		server := NewFileServer(serverOpts)
		transport.OnPeer = server.OnPeer
		servers[i] = server

		// Start the server
		go func(s *FileServer) {
			if err := s.Start(); err != nil {
				t.Logf("Server error: %v", err)
			}
		}(server)
	}

	// Wait for servers to start
	time.Sleep(300 * time.Millisecond)

	// Connect servers
	servers[2].Add(servers[0].Transport.Addr()) // 5000 -> 3000
	time.Sleep(200 * time.Millisecond)
	servers[0].Add(servers[1].Transport.Addr()) // 3000 -> 4000
	time.Sleep(200 * time.Millisecond)
	servers[1].Add(servers[3].Transport.Addr()) // 4000 -> 6000
	time.Sleep(200 * time.Millisecond)

	// Set encryption type
	network.EncType = encType
	network.EncKey = crypto.NewEncryptionKey()
	network.Nonce = crypto.GenerateNonce()
	network.Split = true

	// Wait for network to stabilize
	time.Sleep(500 * time.Millisecond)

	// Return cleanup function
	cleanup := func() {
		// Stop all servers
		for _, server := range servers {
			server.Stop()
		}

		// Reset global network state
		network = Network{} // Reset to zero values

		// Wait for resources to be released
		time.Sleep(200 * time.Millisecond)

		// Clean up directories
		os.RemoveAll(TestStorageRoot)
		os.RemoveAll(TestOutputDir)
	}

	return servers, cleanup
}

// fileChecksum calculates SHA-256 checksum of a file
func fileChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// createTestTextFile creates a large text file with known content
func createTestTextFile(path string, sizeKB int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Create deterministic content with line numbers
	for i := 0; i < sizeKB; i++ {
		line := fmt.Sprintf("Line %d: This is test data line with some content to make it longer.\n", i)
		if _, err := f.WriteString(strings.Repeat(line, 20)); err != nil {
			return err
		}
	}

	return nil
}

// storeAndRetrieveFile tests storing a file on one server and retrieving it from another
func storeAndRetrieveFile(t *testing.T, sourceFile, destFile, fileName string, storeServer, retrieveServer *FileServer) {
	// Open source file
	file, err := os.Open(sourceFile)
	if err != nil {
		t.Fatalf("Failed to open source file: %v", err)
	}
	defer file.Close()

	// Store the file
	cid, err := storeServer.Store(fileName, file)
	if err != nil {
		t.Fatalf("Failed to store file: %v", err)
	}
	t.Logf("Stored file with CID: %s", cid)

	// Allow time for replication
	time.Sleep(500 * time.Millisecond)

	// Retrieve the file
	reader, retrievedName, err := retrieveServer.Get(cid)
	if err != nil {
		t.Fatalf("Failed to retrieve file: %v", err)
	}

	// Check file name
	if retrievedName != fileName {
		t.Errorf("File name mismatch: expected %q, got %q", fileName, retrievedName)
	}

	// Save retrieved file
	outFile, err := os.Create(destFile)
	if err != nil {
		t.Fatalf("Failed to create output file: %v", err)
	}

	written, err := io.Copy(outFile, reader)
	if err != nil {
		outFile.Close()
		t.Fatalf("Failed to write retrieved data: %v", err)
	}
	outFile.Close()

	// Get source file size
	sourceInfo, err := os.Stat(sourceFile)
	if err != nil {
		t.Fatalf("Failed to stat source file: %v", err)
	}

	// Get retrieved file size
	destInfo, err := os.Stat(destFile)
	if err != nil {
		t.Fatalf("Failed to stat destination file: %v", err)
	}

	// Compare sizes
	if sourceInfo.Size() != destInfo.Size() {
		t.Errorf("File size mismatch: source=%d bytes, retrieved=%d bytes", sourceInfo.Size(), written)
	}

	// Compare files thoroughly instead of just checksums
	if !compareFiles(t, sourceFile, destFile) {
		t.Errorf("File content mismatch between source and retrieved files")
	} else {
		// Still show the checksums for reference
		sourceChecksum, _ := fileChecksum(sourceFile)
		// destChecksum, _ := fileChecksum(destFile)
		t.Logf("Files match perfectly (checksum: %s)", sourceChecksum)
	}
}

// TestTextFileWithNoEncryption tests storing and retrieving a text file with no encryption
func TestTextFileWithNoEncryption(t *testing.T) {
	servers, cleanup := setupTestNetwork(t, "None")
	defer cleanup()

	// Create test text file
	sourceFile := filepath.Join(TestOutputDir, "source.txt")
	if err := createTestTextFile(sourceFile, 50); // 50KB text file
	err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test storing on server 0 and retrieving from server 2
	destFile := filepath.Join(TestOutputDir, "retrieved.txt")
	storeAndRetrieveFile(t, sourceFile, destFile, "test.txt", servers[0], servers[2])
}

// TestPDFWithNoEncryption tests storing and retrieving a PDF with no encryption
func TestPDFWithNoEncryption(t *testing.T) {
	servers, cleanup := setupTestNetwork(t, "None")
	defer cleanup()

	// Assume test.pdf exists in the test directory
	sourceFile := "test_tmp/test.pdf"
	if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
		t.Skip("Skipping PDF test: test.pdf not found")
		return
	}

	// Test storing on server 1 and retrieving from server 3
	destFile := filepath.Join(TestOutputDir, "retrieved.pdf")
	storeAndRetrieveFile(t, sourceFile, destFile, "test.pdf", servers[1], servers[3])
}

// TestTextFileWithAESEncryption tests storing and retrieving a text file with AES encryption
func TestTextFileWithAESEncryption(t *testing.T) {
	servers, cleanup := setupTestNetwork(t, "AES")
	defer cleanup()

	// Create test text file
	sourceFile := filepath.Join(TestOutputDir, "source.txt")
	if err := createTestTextFile(sourceFile, 50); // 50KB text file
	err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test storing on server 3 and retrieving from server 1
	destFile := filepath.Join(TestOutputDir, "retrieved.txt")
	storeAndRetrieveFile(t, sourceFile, destFile, "test.txt", servers[3], servers[1])
}

// TestTextFileWithChaCha20Encryption tests storing and retrieving a text file with ChaCha20 encryption
func TestTextFileWithChaCha20Encryption(t *testing.T) {
	servers, cleanup := setupTestNetwork(t, "CC20")
	defer cleanup()

	// Create test text file
	sourceFile := filepath.Join(TestOutputDir, "source.txt")
	if err := createTestTextFile(sourceFile, 50); // 50KB text file
	err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test storing on server 2 and retrieving from server 0
	destFile := filepath.Join(TestOutputDir, "retrieved.txt")
	storeAndRetrieveFile(t, sourceFile, destFile, "test.txt", servers[2], servers[0])
}

// Helper function to compare files byte by byte
func compareFiles(t *testing.T, file1, file2 string) bool {
	// Read both files fully into memory for thorough comparison
	data1, err := os.ReadFile(file1)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", file1, err)
		return false
	}

	data2, err := os.ReadFile(file2)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", file2, err)
		return false
	}

	// Check file sizes
	if len(data1) != len(data2) {
		t.Errorf("File size mismatch: %s has %d bytes, %s has %d bytes",
			file1, len(data1), file2, len(data2))
		return false
	}

	// Compare content
	if !bytes.Equal(data1, data2) {
		t.Errorf("File content mismatch")

		// Find first mismatch
		firstMismatchPos := -1
		for i := 0; i < len(data1); i++ {
			if data1[i] != data2[i] {
				firstMismatchPos = i
				break
			}
		}

		if firstMismatchPos >= 0 {
			// Show context around first mismatch
			start := max(0, firstMismatchPos-10)
			end := min(len(data1), firstMismatchPos+10)
			t.Logf("First difference at byte %d (%.2f%% through file)",
				firstMismatchPos, float64(firstMismatchPos)/float64(len(data1))*100)
			t.Logf("Source bytes around mismatch: %v", data1[start:end])
			t.Logf("Dest bytes around mismatch: %v", data2[start:end])
		}

		// Check end of file - last 50 bytes
		lastBytesCount := 50
		if len(data1) > lastBytesCount && len(data2) > lastBytesCount {
			sourceEnd := data1[len(data1)-lastBytesCount:]
			destEnd := data2[len(data2)-lastBytesCount:]

			if !bytes.Equal(sourceEnd, destEnd) {
				t.Logf("End of file mismatch in last %d bytes:", lastBytesCount)
				t.Logf("Source end bytes: %v", sourceEnd)
				t.Logf("Dest end bytes: %v", destEnd)

				// Find first mismatch position in the end section
				for i := 0; i < lastBytesCount; i++ {
					if sourceEnd[i] != destEnd[i] {
						t.Logf("First end difference at byte %d from end", lastBytesCount-i)
						break
					}
				}
			} else {
				t.Logf("End of files match despite other mismatches")
			}
		}

		// For PDF files, check specific markers
		if strings.HasSuffix(file1, ".pdf") {
			// Check PDF header
			if len(data1) >= 5 && len(data2) >= 5 {
				t.Logf("PDF header comparison: %v vs %v", data1[:5], data2[:5])
			}

			// Check PDF EOF marker
			eofMarker := []byte("%%EOF")
			src := bytes.HasSuffix(data1, eofMarker)
			dst := bytes.HasSuffix(data2, eofMarker)
			t.Logf("PDF EOF marker present: source=%v, dest=%v", src, dst)

			if !src || !dst {
				// Try to find EOF marker within last 1024 bytes
				lastChunk1 := data1[max(0, len(data1)-1024):]
				lastChunk2 := data2[max(0, len(data2)-1024):]

				pos1 := bytes.LastIndex(lastChunk1, eofMarker)
				pos2 := bytes.LastIndex(lastChunk2, eofMarker)

				if pos1 >= 0 && pos2 >= 0 {
					t.Logf("PDF EOF found but not at end: source=%d bytes from end, dest=%d bytes from end",
						len(lastChunk1)-pos1, len(lastChunk2)-pos2)
				} else if pos1 >= 0 {
					t.Logf("PDF EOF only found in source (%d bytes from end)", len(lastChunk1)-pos1)
				} else if pos2 >= 0 {
					t.Logf("PDF EOF only found in dest (%d bytes from end)", len(lastChunk2)-pos2)
				} else {
					t.Logf("PDF EOF marker not found in last 1KB of either file")
				}
			}
		}

		return false
	}

	return true
}
