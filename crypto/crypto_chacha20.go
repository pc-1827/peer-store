package crypto

import (
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20"
)

func GenerateNonce() []byte {
	nonce := make([]byte, chacha20.NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		panic(fmt.Sprintf("failed to generate nonce: %s", err))
	}
	return nonce
}

// ChaCha20Encrypt encrypts data from the src reader and writes the encrypted data to the dst writer
func ChaCha20Encrypt(key, nonce []byte, src io.Reader, dst io.Writer) (int, error) {
	// Create a ChaCha20 cipher with the provided key and nonce
	stream, err := chacha20.NewUnauthenticatedCipher(key, nonce)
	if err != nil {
		return 0, err
	}

	// Wrap the dst writer with a cipher.StreamWriter that encrypts the data
	streamWriter := &cipher.StreamWriter{S: stream, W: dst}

	// Copy the src data to the streamWriter which will encrypt it as it's written to dst
	n, err := io.Copy(streamWriter, src)
	if err != nil {
		return 0, err
	}

	return int(n), nil
}

// ChaCha20Decrypt decrypts the encrypted data from the src reader and writes the decrypted data to the dst writer
func ChaCha20Decrypt(key, nonce []byte, src io.Reader, dst io.Writer) (int, error) {
	// Create a ChaCha20 cipher with the provided key and nonce
	stream, err := chacha20.NewUnauthenticatedCipher(key, nonce)
	if err != nil {
		return 0, err
	}

	// Wrap the dst writer with a cipher.StreamWriter that decrypts the data
	streamWriter := &cipher.StreamWriter{S: stream, W: dst}

	// Copy the encrypted src data to the streamWriter which will decrypt it as it's written to dst
	n, err := io.Copy(streamWriter, src)
	if err != nil {
		return 0, err
	}

	return int(n), nil
}

func copyStream(stream cipher.Stream, src io.Reader, dst io.Writer) (int, error) {
	buf := make([]byte, 32*1024)
	totalBytes := 0
	for {
		n, err := src.Read(buf)
		if n > 0 {
			stream.XORKeyStream(buf[:n], buf[:n])
			nn, err := dst.Write(buf[:n])
			if err != nil {
				return totalBytes, err
			}
			totalBytes += nn
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return totalBytes, err
		}
	}
	return totalBytes, nil
}
