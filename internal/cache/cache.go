package cache

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
)

// Store keeps processed images on disk, keyed by a hex hash.
// A nil *Store is valid and caches nothing.
type Store struct {
	dir string
}

// New parses a cache URL such as "file:///app/cached".
// An empty URL disables caching and returns a nil Store.
func New(rawURL string) (*Store, error) {
	if rawURL == "" {
		return nil, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "file" || u.Path == "" {
		return nil, fmt.Errorf("unsupported cache URL %q: use file:///path", rawURL)
	}
	if err := os.MkdirAll(u.Path, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: u.Path}, nil
}

// Dir returns the cache directory, or "" when caching is disabled.
func (s *Store) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

// path shards files by the first two key characters to keep directories small.
func (s *Store) path(key string) string {
	return filepath.Join(s.dir, key[:2], key)
}

// Get returns the cached data for key, if present.
func (s *Store) Get(key string) ([]byte, bool) {
	if s == nil {
		return nil, false
	}
	data, err := os.ReadFile(s.path(key))
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("cache read error: %v", err)
		}
		return nil, false
	}
	return data, true
}

// Put stores data under key, writing to a temp file first so readers never see partial files.
func (s *Store) Put(key string, data []byte) {
	if s == nil {
		return
	}
	dst := s.path(key)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		log.Printf("cache write error: %v", err)
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), key+".tmp-*")
	if err != nil {
		log.Printf("cache write error: %v", err)
		return
	}
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), dst)
	}
	if err != nil {
		os.Remove(tmp.Name())
		log.Printf("cache write error: %v", err)
	}
}
