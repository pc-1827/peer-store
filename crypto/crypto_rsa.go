package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"fmt"
	"io"
)

func generateKey() *rsa.PrivateKey {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fmt.Println("Error generating a private Key", err)
	}
	return privateKey
}

func encryptRSA(key rsa.PublicKey, src io.Reader, dst io.Writer) (int, error) {
	rng := rand.Reader

	// Read the plaintext from the src reader
	data, err := io.ReadAll(src)
	if err != nil {
		return 0, err
	}

	// Encrypt the plaintext
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rng, &key, data, nil)
	if err != nil {
		return 0, err
	}

	// Write the ciphertext to the dst writer
	n, err := dst.Write(ciphertext)
	if err != nil {
		return 0, err
	}

	return n, nil
}

func decryptRSA(privKey rsa.PrivateKey, src io.Reader, dst io.Writer) (int, error) {
	rng := rand.Reader

	// Read the ciphertext from the src reader
	ciphertext, err := io.ReadAll(src)
	if err != nil {
		return 0, err
	}

	// Decrypt the ciphertext
	data, err := rsa.DecryptOAEP(sha256.New(), rng, &privKey, ciphertext, nil)
	if err != nil {
		return 0, err
	}

	// Write the decrypted data to the dst writer
	n, err := dst.Write(data)
	if err != nil {
		return 0, err
	}

	return n, nil
}
