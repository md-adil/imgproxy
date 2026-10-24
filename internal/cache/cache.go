package cache

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const DefaultValidity = 24 * time.Hour

type Store struct {
	dir      string
	validity time.Duration
}

func New(rawURL string) (*Store, error) {
	if rawURL == "" {
		return nil, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	dir := u.Path
	switch {
	case u.Opaque != "":
		dir = u.Opaque
	case u.Host != "" && u.Host != "localhost":
		dir = u.Host + u.Path
	}
	if u.Scheme != "file" || dir == "" {
		return nil, fmt.Errorf("unsupported cache URL %q: use file:///abs/path or file://./rel/path", rawURL)
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return nil, err
	}
	validity := DefaultValidity
	if v := u.Query().Get("validity"); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs <= 0 {
			return nil, fmt.Errorf("invalid cache validity %q: use a positive number of seconds", v)
		}
		validity = time.Duration(secs) * time.Second
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, validity: validity}
	go s.cleanup()
	return s, nil
}

func (s *Store) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

func (s *Store) Validity() time.Duration {
	if s == nil {
		return 0
	}
	return s.validity
}

func (s *Store) path(key string) string {
	return filepath.Join(s.dir, key[:2], key)
}

func (s *Store) expired(info fs.FileInfo) bool {
	return time.Since(info.ModTime()) > s.validity
}

func (s *Store) Get(key string) ([]byte, bool) {
	if s == nil || len(key) < 2 {
		return nil, false
	}
	p := s.path(key)
	f, err := os.Open(p)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("cache read error: %v", err)
		}
		return nil, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		log.Printf("cache read error: %v", err)
		return nil, false
	}
	if s.expired(info) {
		os.Remove(p)
		return nil, false
	}
	data, err := io.ReadAll(f)
	if err != nil {
		log.Printf("cache read error: %v", err)
		return nil, false
	}
	return data, true
}

func (s *Store) Put(key string, data []byte) {
	if s == nil || len(key) < 2 {
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

func (s *Store) cleanup() {
	interval := min(max(s.validity/2, time.Minute), time.Hour)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		s.walk()
	}
}

func (s *Store) walk() {
	removed := 0
	filepath.WalkDir(s.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err == nil && s.expired(info) && os.Remove(p) == nil {
			removed++
		}
		return nil
	})
	if removed > 0 {
		log.Printf("cache cleanup: removed %d expired files", removed)
	}
}
