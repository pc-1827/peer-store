package main

import (
	"bytes"
	"crypto/sha1"
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

func (s *Store) Has(id string, key string) bool {
	pathKey := s.PathTransformFunc(key)
	FullPathNameWithRoot := fmt.Sprintf("%s/%s/%s", s.Root, id, pathKey.FullPathName())
	_, err := os.Stat(FullPathNameWithRoot)
	return !errors.Is(err, os.ErrNotExist)
}

func (s *Store) Delete(id string, key string) error {
	pathKey := s.PathTransformFunc(key)

	defer func() {
		log.Printf("deleted [%s] from disk", pathKey.FileName)
	}()

	firstPathNameWithRoot := fmt.Sprintf("%s/%s/%s", s.Root, id, pathKey.FirstPathName())

	return os.RemoveAll(firstPathNameWithRoot)
}

func (s *Store) Read(id string, key string) (int64, io.Reader, error) {
	return s.readStream(id, key)
}

func (s *Store) ReadDecrypt(fs *FileServer, id string, key string) (int64, io.Reader, string, error) {
	// Read and decrypt the file data
	_, r, err := s.readStream(id, key)
	if err != nil {
		return 0, nil, "", err
	}

	fileBuffer := new(bytes.Buffer)
	n, err := fs.decrypt(r, fileBuffer)
	if err != nil {
		return 0, nil, "", err
	}

	// Read and decrypt the metadata
	metadata, err := s.readMetadata(fs, id, key)
	if err != nil {
		return 0, nil, "", err
	}

	return int64(n), fileBuffer, metadata.FileName, nil
}

func (s *Store) readMetadata(fs *FileServer, id string, key string) (Metadata, error) {
	pathKey := s.PathTransformFunc(key)
	FullPathNameWithRoot := fmt.Sprintf("%s/%s/%s/metadata", s.Root, id, pathKey.PathName)

	metadataFile, err := os.Open(FullPathNameWithRoot)
	if err != nil {
		return Metadata{}, err
	}
	defer metadataFile.Close()

	time.Sleep(100 * time.Millisecond)

	var metadataString string
	if fs != nil {
		metadataBuffer := new(bytes.Buffer)
		if _, err := fs.decrypt(metadataFile, metadataBuffer); err != nil {
			return Metadata{}, err
		}
		metadataString = metadataBuffer.String()
	} else {
		metadataBytes, err := io.ReadAll(metadataFile)
		if err != nil {
			return Metadata{}, err
		}
		metadataString = string(metadataBytes)
	}

	fmt.Println(metadataString)
	metadataParts := strings.Split(metadataString, ",")
	if len(metadataParts) != 3 {
		return Metadata{}, errors.New("invalid metadata format")
	}

	metadata := Metadata{
		ServerID: metadataParts[0],
		FileID:   metadataParts[1],
		FileName: metadataParts[2],
	}
	return metadata, nil
}

// func (s *Store) ReadDecrypt(fs *FileServer, id string, key string) (int64, io.Reader, error) {
// 	_, r, err := s.readStream(id, key)
// 	if err != nil {
// 		return 0, nil, err
// 	}

// 	fileBuffer := new(bytes.Buffer)
// 	n, err := fs.decrypt(r, fileBuffer)
// 	return int64(n), fileBuffer, err
// }

func (s *Store) readStream(id string, key string) (int64, io.ReadCloser, error) {
	pathKey := s.PathTransformFunc(key)
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
	n, err := s.writeStream(id, key, r)
	if err != nil {
		return 0, err
	}

	if err := s.writeMetadata(nil, id, key); err != nil {
		return 0, err
	}

	return n, nil
}

func (s *Store) WriteEncrypt(fs *FileServer, id string, key string, r io.Reader) (int64, error) {
	// Write the file data
	f, err := s.openFileForWriting(id, key)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	n, err := fs.encrypt(r, f)
	if err != nil {
		return 0, err
	}

	fmt.Println("Hello from the WriteEncrypt func after writing stream")

	// Write the metadata

	if err := s.writeMetadata(fs, id, key); err != nil {
		return 0, err
	}

	return int64(n), nil
}

// func (s *Store) WriteEncrypt(fs *FileServer, id string, key string, r io.Reader) (int64, error) {
// 	f, err := s.openFileForWriting(id, key)
// 	if err != nil {
// 		return 0, err
// 	}

// 	b := bytes.NewBuffer()

// 	n, err := fs.encrypt(r, f)
// 	return int64(n), err
// }

func (s *Store) writeMetadata(fs *FileServer, id string, key string) error {
	fmt.Println("Hello from writeMetadata func")
	pathKey := s.PathTransformFunc(key)
	FullPathNameWithRoot := fmt.Sprintf("%s/%s/%s/metadata", s.Root, id, pathKey.PathName)

	metadataFile, err := os.Create(FullPathNameWithRoot)
	if err != nil {
		return err
	}
	defer metadataFile.Close()

	time.Sleep(100 * time.Millisecond)

	fmt.Println("Hello from writeMetadata func 2")

	metadata := Metadata{
		ServerID: id,
		FileID:   pathKey.FileName,
		FileName: key,
	}

	metadataString := fmt.Sprintf("%s,%s,%s", metadata.ServerID, metadata.FileID, metadata.FileName)
	metadataBuffer := bytes.NewBufferString(metadataString)

	if fs != nil {
		if _, err := fs.encrypt(metadataBuffer, metadataFile); err != nil {
			return err
		}
	} else {
		if _, err := io.Copy(metadataFile, metadataBuffer); err != nil {
			return err
		}
	}

	fmt.Println("Hello from writeMetadata func 3")

	return nil
}

func (s *Store) openFileForWriting(id string, key string) (*os.File, error) {
	pathKey := s.PathTransformFunc(key)
	PathNameWithRoot := fmt.Sprintf("%s/%s/%s", s.Root, id, pathKey.PathName)

	if err := os.MkdirAll(PathNameWithRoot, os.ModePerm); err != nil {
		return nil, err
	}

	FullPathNameWithRoot := fmt.Sprintf("%s/%s/%s", s.Root, id, pathKey.FullPathName())

	return os.Create(FullPathNameWithRoot)
}

func (s *Store) writeStream(id string, key string, r io.Reader) (int64, error) {
	f, err := s.openFileForWriting(id, key)
	if err != nil {
		return 0, err
	}

	return io.Copy(f, r)
}
