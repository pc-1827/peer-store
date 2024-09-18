package main

import (
	"bytes"
	"fmt"
	"io"
	"testing"
)

func TestPathTransformFunc(t *testing.T) {
	key := "someimportantdata"
	pathKey := CASPathTransformFunc(key)
	expectedFileName := "349d8ede57e318d842787b6c9ac811a6dfb0dc7a"
	expectedPathName := "349d8/ede57/e318d/84278/7b6c9/ac811/a6dfb/0dc7a"
	if expectedPathName != pathKey.PathName {
		t.Fatalf("expected %s, got %s", expectedPathName, pathKey.PathName)
	}
	if expectedFileName != pathKey.FileName {
		t.Fatalf("expected %s, got %s", expectedFileName, pathKey.FileName)
	}
}

func TestStoreDelete(t *testing.T) {
	opts := StoreOptions{
		PathTransformFunc: CASPathTransformFunc,
	}
	s := NewStore(opts)

	key := "myspecialdata"

	data := []byte("some data")
	if err := s.writeStream(key, bytes.NewReader(data)); err != nil {
		t.Fatalf("writeStream failed: %v", err)
	}

	if err := s.Delete(key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, err := s.Read(key); err == nil {
		t.Fatalf("Read should have failed")
	}
}

func TestStore(t *testing.T) {
	opts := StoreOptions{
		PathTransformFunc: CASPathTransformFunc,
	}
	s := NewStore(opts)

	key := "myspecialdata"

	data := []byte("some data")
	if err := s.writeStream(key, bytes.NewReader(data)); err != nil {
		t.Fatalf("writeStream failed: %v", err)
	}

	r, err := s.Read(key)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	b, _ := io.ReadAll(r)
	fmt.Println(string(b))

	if string(b) != string(data) {
		t.Fatalf("expected %s, got %s", string(data), string(b))
	}

	s.Delete(key)
}
