package modules

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

type mockKeyValueStore struct {
	data map[string][]string
}

func (m *mockKeyValueStore) Get(key string) ([]string, bool) {
	v, ok := m.data[key]
	return v, ok
}

func (m *mockKeyValueStore) Set(key string, value []string, d ...time.Duration) {
	m.data[key] = value
}

type mockFileStorage struct {
	files map[string][]byte
}

func (m *mockFileStorage) Open(filename string) (io.ReadCloser, error) {
	data, ok := m.files[filename]
	if !ok {
		return nil, errors.New("file not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *mockFileStorage) Create(filename string) (io.WriteCloser, error) {
	var buf bytes.Buffer
	m.files[filename] = buf.Bytes()
	return nopWriteCloser{&buf, func() { m.files[filename] = buf.Bytes() }}, nil
}

type nopWriteCloser struct {
	*bytes.Buffer
	closeFunc func()
}

func (n nopWriteCloser) Close() error {
	n.closeFunc()
	return nil
}

type mockCacheRepository struct {
	versions map[string][]string
}

func (m *mockCacheRepository) ListVersions(ctx context.Context, owner, repo, module string) ([]string, error) {
	key := owner + "/" + repo + "/" + module
	v, ok := m.versions[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return v, nil
}

func (m *mockCacheRepository) ProxyDownload(ctx context.Context, owner, repo, module, version string, w io.Writer) error {
	_, err := w.Write([]byte("fake tarball content"))
	return err
}

type mockLogger struct{}

func (m *mockLogger) Error(msg string, keysAndValues ...any) {}
func (m *mockLogger) Info(msg string, keysAndValues ...any)  {}

func TestCache_ListVersions(t *testing.T) {
	store := &mockKeyValueStore{data: make(map[string][]string)}
	repo := &mockCacheRepository{versions: map[string][]string{
		"owner/repo/module": {"v1.0.0", "v1.1.0"},
	}}
	cache := NewCache(repo, store, nil, nil)

	versions, err := cache.ListVersions(context.Background(), "owner", "repo", "module")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"v1.0.0", "v1.1.0"}
	if len(versions) != len(expected) {
		t.Fatalf("expected %d versions, got %d", len(expected), len(versions))
	}
	for i, v := range versions {
		if v != expected[i] {
			t.Errorf("expected version %q, got %q", expected[i], v)
		}
	}
}

func TestCache_ProxyDownload(t *testing.T) {
	store := &mockKeyValueStore{data: make(map[string][]string)}
	files := &mockFileStorage{files: make(map[string][]byte)}
	repo := &mockCacheRepository{}
	logger := &mockLogger{}
	cache := NewCache(repo, store, files, logger)

	var buf bytes.Buffer
	err := cache.ProxyDownload(context.Background(), "owner", "repo", "module", "v1.0.0", &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedContent := "fake tarball content"
	if buf.String() != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, buf.String())
	}
}
