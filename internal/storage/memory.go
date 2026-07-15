package storage

import (
	"bytes"
	"context"
	"io"
	"sync"
)

// Memory is an in-process blob store for local development and tests.
type Memory struct {
	mu   sync.RWMutex
	data map[string]memObj
}

type memObj struct {
	body        []byte
	contentType string
}

func NewMemory() *Memory {
	return &Memory{data: map[string]memObj{}}
}

func (m *Memory) Put(_ context.Context, key string, body io.Reader, _ int64, contentType string) error {
	b, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.data[key] = memObj{body: b, contentType: contentType}
	m.mu.Unlock()
	return nil
}

func (m *Memory) Get(_ context.Context, key string) (io.ReadCloser, string, error) {
	m.mu.RLock()
	obj, ok := m.data[key]
	m.mu.RUnlock()
	if !ok {
		return nil, "", ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(obj.body)), obj.contentType, nil
}

func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.data, key)
	m.mu.Unlock()
	return nil
}
