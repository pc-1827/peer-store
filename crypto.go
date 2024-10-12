package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/pc-1827/distributed-file-system/crypto"
)

func GenerateID() string {
	buf := make([]byte, 32)
	io.ReadFull(rand.Reader, buf)
	return hex.EncodeToString(buf)
}

func encrypt(src io.Reader, dst io.Writer) (int, error) {
	if network.EncType == "AES" {
		n, err := crypto.EncryptAES(network.EncKey, src, dst)
		if err != nil {
			return 0, fmt.Errorf("failed to encrypt data: %s", err)
		}
		return n, nil
	} else if network.EncType == "CC20" {
		//nonce := crypto.GenerateNonce()
		n, err := crypto.ChaCha20Encrypt(network.EncKey, network.Nonce, src, dst)
		if err != nil {
			return 0, fmt.Errorf("failed to encrypt data: %s", err)
		}
		return n, nil
	}
	return 0, nil
}

func decrypt(src io.Reader, dst io.Writer) (int, error) {
	if network.EncType == "AES" {
		n, err := crypto.DecryptAES(network.EncKey, src, dst)
		if err != nil {
			return 0, fmt.Errorf("failed to decrypt data: %s", err)
		}
		return n, nil
	} else if network.EncType == "CC20" {
		n, err := crypto.ChaCha20Decrypt(network.EncKey, network.Nonce, src, dst)
		if err != nil {
			return 0, fmt.Errorf("failed to decrypt data: %s", err)
		}
		return n, nil
	}
	return 0, nil
}
