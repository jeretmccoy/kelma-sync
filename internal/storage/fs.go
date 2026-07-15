package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotFound is returned by a Storage.Get when the object does not exist.
// Handlers translate this into an HTTP 404 (rather than a 500) so clients can
// treat the file as absent and re-upload their local copy.
var ErrNotFound = errors.New("storage: object not found")

// FS is a filesystem-backed Storage for local development and self-hosting.
// Unlike Memory it survives server restarts, so pushed media persists.
type FS struct {
	root string
}

// NewFS creates (if needed) and returns a filesystem store rooted at dir.
func NewFS(dir string) (*FS, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	return &FS{root: abs}, nil
}

// path maps a storage key ("<userID>/<filename>") to an on-disk path, guarding
// against path traversal.
func (f *FS) path(key string) (string, error) {
	clean := filepath.Clean("/" + key) // force absolute, collapse ../
	p := filepath.Join(f.root, clean)
	if p != f.root && !strings.HasPrefix(p, f.root+string(os.PathSeparator)) {
		return "", errors.New("storage: key escapes root")
	}
	return p, nil
}

func (f *FS) Put(_ context.Context, key string, body io.Reader, _ int64, contentType string) error {
	p, err := f.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, body); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	// Persist the content type alongside the blob so Get can return it.
	if contentType != "" {
		_ = os.WriteFile(p+".ct", []byte(contentType), 0o644)
	}
	return os.Rename(tmp, p)
}

func (f *FS) Get(_ context.Context, key string) (io.ReadCloser, string, error) {
	p, err := f.path(key)
	if err != nil {
		return nil, "", err
	}
	file, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	ct := ""
	if b, err := os.ReadFile(p + ".ct"); err == nil {
		ct = string(b)
	}
	return file, ct, nil
}

func (f *FS) Delete(_ context.Context, key string) error {
	p, err := f.path(key)
	if err != nil {
		return err
	}
	_ = os.Remove(p + ".ct")
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
