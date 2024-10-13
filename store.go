package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

const DefaultRootNameFolder = "rootStore"

func CASPathTransformFunc(key string) PathKey {
	hash := sha1.Sum([]byte(key))
	hashStr := hex.EncodeToString(hash[:])

	blockSize := 5
	sliceLen := len(hashStr) / blockSize
	paths := make([]string, sliceLen)

	for i := 0; i < sliceLen; i++ {
		from, to := i*blockSize, (i*blockSize)+blockSize
		paths[i] = hashStr[from:to]
	}

	return PathKey{
		PathName: strings.Join(paths, "/"),
		FileName: hashStr,
	}
}

type PathTransformFunc func(string) PathKey

type PathKey struct {
	PathName string
	FileName string
}

type Metadata struct {
	ServerID string
	FileID   string
	FileName string
}

func (p PathKey) FullPathName() string {
	return fmt.Sprintf("%s/%s", p.PathName, p.FileName)
}

func (p PathKey) FirstPathName() string {
	paths := strings.Split(p.PathName, "/")
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

type StoreOptions struct {
	Root              string
	PathTransformFunc PathTransformFunc
}

var DefaultPathTransformFunc = func(key string) PathKey {
	return PathKey{
		PathName: key,
		FileName: key,
	}
}

type Store struct {
	StoreOptions
}

func NewStore(opts StoreOptions) *Store {
	if opts.PathTransformFunc == nil {
		opts.PathTransformFunc = DefaultPathTransformFunc
	}
	if len(opts.Root) == 0 {
		opts.Root = DefaultRootNameFolder
	}
	return &Store{
		StoreOptions: opts,
	}
}

func (s *Store) Clear() error {
	return os.RemoveAll(s.Root)
}

func (s *Store) Has(id string) bool {
	pathKey := s.PathTransformFunc(id)
	FullPathNameWithRoot := fmt.Sprintf("%s/%s/%s", s.Root, id, pathKey.FullPathName())
	_, err := os.Stat(FullPathNameWithRoot)
	return !errors.Is(err, os.ErrNotExist)
}

func (s *Store) Delete(id string) error {
	//pathKey := s.PathTransformFunc(id)

	defer func() {
		log.Printf("deleted [%s] from disk", id)
	}()

	firstPathNameWithRoot := fmt.Sprintf("%s/%s", s.Root, id)

	return os.RemoveAll(firstPathNameWithRoot)
}

func (s *Store) Read(id string) (int64, io.Reader, error) {
	return s.readStream(id)
}

func (s *Store) ReadDecrypt(id string) (int64, io.Reader, string, error) {
	_, r, err := s.readStream(id)
	if err != nil {
		return 0, nil, "", err
	}

	fileBuffer := new(bytes.Buffer)
	n, err := decrypt(r, fileBuffer)
	if err != nil {
		return 0, nil, "", err
	}

	metadata, err := s.readMetadata(id)
	if err != nil {
		return 0, nil, "", err
	}

	return int64(n), fileBuffer, metadata.FileName, nil
}

func (s *Store) readMetadata(id string) (Metadata, error) {
	pathKey := s.PathTransformFunc(id)
	FullPathNameWithRoot := fmt.Sprintf("%s/%s/%s/metadata", s.Root, id, pathKey.PathName)

	metadataFile, err := os.Open(FullPathNameWithRoot)
	if err != nil {
		return Metadata{}, err
	}
	defer metadataFile.Close()

	time.Sleep(100 * time.Millisecond)

	metadataBuffer := new(bytes.Buffer)
	if _, err := decrypt(metadataFile, metadataBuffer); err != nil {
		return Metadata{}, err
	}

	var metadata Metadata
	if err := gob.NewDecoder(metadataBuffer).Decode(&metadata); err != nil {
		return Metadata{}, err
	}

	return metadata, nil
}

func (s *Store) readStream(id string) (int64, io.ReadCloser, error) {
	pathKey := s.PathTransformFunc(id)
	FullPathNameWithRoot := fmt.Sprintf("%s/%s/%s", s.Root, id, pathKey.FullPathName())

	file, err := os.Open(FullPathNameWithRoot)
	if err != nil {
		return 0, nil, err
	}

	fi, err := file.Stat()
	if err != nil {
		return 0, nil, err
	}

	return fi.Size(), file, nil
}

func (s *Store) Write(id string, key string, r io.Reader) (int64, error) {
	n, err := s.writeStream(id, r)
	if err != nil {
		return 0, err
	}

	if err := s.writeMetadata(id, key); err != nil {
		return 0, err
	}

	return n, nil
}

func (s *Store) WriteEncrypt(id string, key string, r io.Reader) (int64, error) {
	f, err := s.openFileForWriting(id)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	n, err := encrypt(r, f)
	if err != nil {
		return 0, err
	}

	if err := s.writeMetadata(id, key); err != nil {
		return 0, err
	}

	return int64(n), nil
}

func (s *Store) writeMetadata(id string, key string) error {
	pathKey := s.PathTransformFunc(id)
	FullPathNameWithRoot := fmt.Sprintf("%s/%s/%s/metadata", s.Root, id, pathKey.PathName)

	metadataFile, err := os.Create(FullPathNameWithRoot)
	if err != nil {
		return err
	}
	defer metadataFile.Close()

	time.Sleep(100 * time.Millisecond)

	metadata := Metadata{
		ServerID: id,
		FileID:   pathKey.FileName,
		FileName: key,
	}

	metadataBuffer := new(bytes.Buffer)
	if err := gob.NewEncoder(metadataBuffer).Encode(metadata); err != nil {
		return err
	}

	if _, err := encrypt(metadataBuffer, metadataFile); err != nil {
		return err
	}

	return nil
}

func (s *Store) openFileForWriting(id string) (*os.File, error) {
	pathKey := s.PathTransformFunc(id)
	PathNameWithRoot := fmt.Sprintf("%s/%s/%s", s.Root, id, pathKey.PathName)

	if err := os.MkdirAll(PathNameWithRoot, os.ModePerm); err != nil {
		return nil, err
	}

	FullPathNameWithRoot := fmt.Sprintf("%s/%s/%s", s.Root, id, pathKey.FullPathName())

	return os.Create(FullPathNameWithRoot)
}

func (s *Store) writeStream(id string, r io.Reader) (int64, error) {
	f, err := s.openFileForWriting(id)
	if err != nil {
		return 0, err
	}

	return io.Copy(f, r)
}
