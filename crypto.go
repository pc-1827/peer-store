package main

import (
	"fmt"
	"io"

	"github.com/pc-1827/distributed-file-system/crypto"
)

func (s *FileServer) encrypt(src io.Reader, dst io.Writer) (int, error) {
	if s.EncType == "AES" {
		n, err := crypto.EncryptAES(s.EncKey, src, dst)
		if err != nil {
			return 0, fmt.Errorf("failed to encrypt data: %s", err)
		}
		return n, nil
	} else if s.EncType == "CC20" {
		//nonce := crypto.GenerateNonce()
		n, err := crypto.ChaCha20Encrypt(s.EncKey, s.Nonce, src, dst)
		if err != nil {
			return 0, fmt.Errorf("failed to encrypt data: %s", err)
		}
		return n, nil
	}
	return 0, nil
}

func (s *FileServer) decrypt(src io.Reader, dst io.Writer) (int, error) {
	if s.EncType == "AES" {
		n, err := crypto.DecryptAES(s.EncKey, src, dst)
		if err != nil {
			return 0, fmt.Errorf("failed to decrypt data: %s", err)
		}
		return n, nil
	} else if s.EncType == "CC20" {
		n, err := crypto.ChaCha20Decrypt(s.EncKey, s.Nonce, src, dst)
		if err != nil {
			return 0, fmt.Errorf("failed to decrypt data: %s", err)
		}
		return n, nil
	}
	return 0, nil
}
