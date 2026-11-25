package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"vedam/lms/internal/domain/errs"
)

// Root confines storage keys to the configured directory, including symlinks.
type LocalAssets struct{ root *os.Root }

func NewLocalAssets(dir string) (*LocalAssets, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &LocalAssets{root}, nil
}
func (s *LocalAssets) Close() error { return s.root.Close() }
func (s *LocalAssets) Put(ctx context.Context, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsLocal(key) {
		return errs.ErrValidation
	}
	if err := s.root.MkdirAll(filepath.Dir(key), 0700); err != nil {
		return err
	}
	f, err := s.root.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	err = errors.Join(writeErr, f.Close())
	if err != nil {
		_ = s.root.Remove(key)
	}
	return err
}
func (s *LocalAssets) Open(ctx context.Context, key string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := s.root.Open(key)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errs.ErrNotFound
	}
	return f, err
}
func (s *LocalAssets) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.root.ReadFile(key)
}
func (s *LocalAssets) Remove(key string) error { return s.root.Remove(key) }
