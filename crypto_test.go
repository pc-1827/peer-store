package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pc-1827/peer-store/crypto"
)

// TestEncryptDecryptNone tests the "None" encryption type
func TestEncryptDecryptNone(t *testing.T) {
	// Save original network settings
	originalNetwork := network
	defer func() { network = originalNetwork }()

	// Set up "None" encryption
	network.EncType = "None"

	// Test with various data sizes
	testCases := []struct {
		name string
		data string
	}{
		{"Empty", ""},
		{"Small", "Hello, world!"},
		{"Medium", strings.Repeat("The quick brown fox jumps over the lazy dog. ", 20)},
		{"Large", strings.Repeat("Lorem ipsum dolor sit amet. ", 1000)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up source and destination buffers
			src := bytes.NewReader([]byte(tc.data))
			encBuf := new(bytes.Buffer)

			// Encrypt
			n, err := encrypt(src, encBuf)
			if err != nil {
				t.Fatalf("encrypt failed: %v", err)
			}

			// Check that encrypted size matches input size for "None" type
			if n != len(tc.data) {
				t.Errorf("encrypt returned wrong size: got %d, want %d", n, len(tc.data))
			}

			// Check that with "None" encryption, the data is unchanged
			if encBuf.String() != tc.data {
				t.Errorf("None encryption modified data: got %q, want %q", encBuf.String(), tc.data)
			}

			// Decrypt
			decBuf := new(bytes.Buffer)
			_, err = decrypt(encBuf, decBuf)
			if err != nil {
				t.Fatalf("decrypt failed: %v", err)
			}

			// Verify decrypted data matches original
			if decBuf.String() != tc.data {
				t.Errorf("decrypt produced wrong data: got %q, want %q", decBuf.String(), tc.data)
			}
		})
	}
}

// TestEncryptDecryptAES tests the AES encryption type
func TestEncryptDecryptAES(t *testing.T) {
	// Save original network settings
	originalNetwork := network
	defer func() { network = originalNetwork }()

	// Set up AES encryption
	network.EncType = "AES"
	network.EncKey = crypto.NewEncryptionKey()

	// Test with various data sizes
	testCases := []struct {
		name string
		data string
	}{
		{"Small", "Hello, world!"},
		{"Medium", strings.Repeat("The quick brown fox jumps over the lazy dog. ", 20)},
		{"Large", strings.Repeat("Lorem ipsum dolor sit amet. ", 100)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up source and destination buffers
			src := bytes.NewReader([]byte(tc.data))
			encBuf := new(bytes.Buffer)

			// Encrypt
			n, err := encrypt(src, encBuf)
			if err != nil {
				t.Fatalf("AES encrypt failed: %v", err)
			}

			// Check that encrypted data is different from input
			if encBuf.String() == tc.data {
				t.Error("AES encryption didn't change the data")
			}

			// Check that encrypted size is non-zero
			if n == 0 {
				t.Error("AES encrypt returned 0 bytes")
			}

			// Decrypt
			decBuf := new(bytes.Buffer)
			_, err = decrypt(encBuf, decBuf)
			if err != nil {
				t.Fatalf("AES decrypt failed: %v", err)
			}

			// Verify decrypted data matches original
			if decBuf.String() != tc.data {
				t.Errorf("AES decrypt produced wrong data: got %q, want %q", decBuf.String(), tc.data)
			}
		})
	}
}

// TestEncryptDecryptChaCha20 tests the ChaCha20 encryption type
func TestEncryptDecryptChaCha20(t *testing.T) {
	// Save original network settings
	originalNetwork := network
	defer func() { network = originalNetwork }()

	// Set up ChaCha20 encryption
	network.EncType = "CC20"
	network.EncKey = crypto.NewEncryptionKey()
	network.Nonce = crypto.GenerateNonce()

	// Test with various data sizes
	testCases := []struct {
		name string
		data string
	}{
		{"Small", "Hello, world!"},
		{"Medium", strings.Repeat("The quick brown fox jumps over the lazy dog. ", 20)},
		{"Large", strings.Repeat("Lorem ipsum dolor sit amet. ", 100)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up source and destination buffers
			src := bytes.NewReader([]byte(tc.data))
			encBuf := new(bytes.Buffer)

			// Encrypt
			n, err := encrypt(src, encBuf)
			if err != nil {
				t.Fatalf("ChaCha20 encrypt failed: %v", err)
			}

			// Check that encrypted data is different from input
			if encBuf.String() == tc.data {
				t.Error("ChaCha20 encryption didn't change the data")
			}

			// Check that encrypted size is non-zero
			if n == 0 {
				t.Error("ChaCha20 encrypt returned 0 bytes")
			}

			// Decrypt
			decBuf := new(bytes.Buffer)
			_, err = decrypt(encBuf, decBuf)
			if err != nil {
				t.Fatalf("ChaCha20 decrypt failed: %v", err)
			}

			// Verify decrypted data matches original
			if decBuf.String() != tc.data {
				t.Errorf("ChaCha20 decrypt produced wrong data: got %q, want %q", decBuf.String(), tc.data)
			}
		})
	}
}

// TestDifferentKeys tests that different keys produce different encrypted outputs
func TestDifferentKeys(t *testing.T) {
	// Save original network settings
	originalNetwork := network
	defer func() { network = originalNetwork }()

	plaintext := "This is a test message for encryption"

	// Test AES with different keys
	t.Run("AES_DifferentKeys", func(t *testing.T) {
		network.EncType = "AES"
		network.EncKey = crypto.NewEncryptionKey()

		buf1 := new(bytes.Buffer)
		_, err := encrypt(strings.NewReader(plaintext), buf1)
		if err != nil {
			t.Fatalf("First encryption failed: %v", err)
		}

		// Use a different key
		network.EncKey = crypto.NewEncryptionKey()

		buf2 := new(bytes.Buffer)
		_, err = encrypt(strings.NewReader(plaintext), buf2)
		if err != nil {
			t.Fatalf("Second encryption failed: %v", err)
		}

		// The two encrypted outputs should be different
		if bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
			t.Error("Different AES keys produced identical encrypted output")
		}
	})

	// Test ChaCha20 with different keys
	t.Run("ChaCha20_DifferentKeys", func(t *testing.T) {
		network.EncType = "CC20"
		network.EncKey = crypto.NewEncryptionKey()
		network.Nonce = crypto.GenerateNonce()

		buf1 := new(bytes.Buffer)
		_, err := encrypt(strings.NewReader(plaintext), buf1)
		if err != nil {
			t.Fatalf("First encryption failed: %v", err)
		}

		// Use a different key but same nonce
		network.EncKey = crypto.NewEncryptionKey()

		buf2 := new(bytes.Buffer)
		_, err = encrypt(strings.NewReader(plaintext), buf2)
		if err != nil {
			t.Fatalf("Second encryption failed: %v", err)
		}

		// The two encrypted outputs should be different
		if bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
			t.Error("Different ChaCha20 keys produced identical encrypted output")
		}
	})
}

// TestGenerateCID tests that the CID generation is consistent
func TestGenerateCID(t *testing.T) {
	data1 := "Test data for CID generation"
	data2 := "Different test data"

	// Generate CID for the same data twice
	cid1 := GenerateCID(strings.NewReader(data1))
	cid2 := GenerateCID(strings.NewReader(data1))

	// They should be the same
	if cid1 != cid2 {
		t.Errorf("CID generation not consistent: %s != %s", cid1, cid2)
	}

	// Generate CID for different data
	cid3 := GenerateCID(strings.NewReader(data2))

	// It should be different
	if cid1 == cid3 {
		t.Errorf("CID generation produced same hash for different data: %s", cid1)
	}
}
