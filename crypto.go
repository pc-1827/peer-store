package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"

	"github.com/pc-1827/peer-store/crypto"
)

func GenerateCID(r io.Reader) string {
	// Generate CID by hashing the data
	hash := sha1.New()
	if _, err := io.Copy(hash, r); err != nil {
		log.Fatal(err)
		return ""
	}
	return hex.EncodeToString(hash.Sum(nil))
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
	} else if network.EncType == "None" {
		n, err := io.Copy(dst, src)
		if err != nil {
			return 0, fmt.Errorf("failed to encrypt data: %s", err)
		}
		return int(n), nil
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
	} else if network.EncType == "None" {
		n, err := io.Copy(dst, src)
		if err != nil {
			return 0, fmt.Errorf("failed to decrypt data: %s", err)
		}
		return int(n), nil
	}
	return 0, nil
}
