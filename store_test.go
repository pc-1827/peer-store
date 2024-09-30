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

func TestStore(t *testing.T) {
	s := newStore()
	id := generateID()
	defer teardown(t, s)

	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("myspecialdata_%d", i)

		data := []byte("some data")
		_, err := s.writeStream(id, key, bytes.NewReader(data))
		if err != nil {
			t.Fatalf("writeStream failed: %v", err)
		}

		ok := s.Has(id, key)
		if !ok {
			t.Fatalf("Key not found")
		}

		_, r, err := s.Read(id, key)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		b, _ := io.ReadAll(r)
		fmt.Println(string(b))
		if string(b) != string(data) {
			t.Fatalf("expected %s, got %s", string(data), string(b))
		}

		if err := s.Delete(id, key); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		if ok := s.Has(id, key); ok {
			t.Fatalf("Key should have been deleted")
		}
	}
}

func newStore() *Store {
	opts := StoreOptions{
		PathTransformFunc: CASPathTransformFunc,
	}
	return NewStore(opts)
}

func teardown(t *testing.T, s *Store) {
	t.Helper()
	if err := s.Clear(); err != nil {
		t.Error(err)
	}
}
