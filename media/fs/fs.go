// Package fs implements media.Store using the local filesystem.
// ローカルファイルシステムによる media.Store の実装。
package fs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/murlog-org/murlog/media"
)

type fsStore struct {
	dir     string
	baseURL string
}

// New creates a filesystem-backed media store.
// ファイルシステムバックエンドのメディアストアを生成する。
func New(dir, baseURL string) media.Store {
	return &fsStore{dir: dir, baseURL: baseURL}
}

// safePath resolves name within dir and ensures it doesn't escape (path traversal prevention).
// name を dir 内で解決し、外部に出ないことを保証する (パストラバーサル防止)。
func (s *fsStore) safePath(name string) (string, error) {
	p := filepath.Join(s.dir, filepath.Clean(name))
	if !strings.HasPrefix(p, filepath.Clean(s.dir)+string(filepath.Separator)) {
		return "", fmt.Errorf("fs: path traversal detected: %s", name)
	}
	return p, nil
}

func (s *fsStore) Save(name string, r io.Reader) error {
	path, err := s.safePath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, r)
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(path)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(path)
		return closeErr
	}
	return nil
}

func (s *fsStore) Open(name string) (io.ReadCloser, error) {
	path, err := s.safePath(name)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (s *fsStore) Delete(name string) error {
	path, err := s.safePath(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (s *fsStore) URL(name string) string {
	return s.baseURL + "/" + name
}
