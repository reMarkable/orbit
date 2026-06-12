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
	versions    map[string][]string
	repoHeadSet bool
	repoHeadErr error
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

func (m *mockCacheRepository) RepoHead(ctx context.Context, owner, repo string) error {
	if m.repoHeadSet {
		return m.repoHeadErr
	}
	prefix := owner + "/" + repo + "/"
	for k := range m.versions {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			return nil
		}
	}
	return errors.New("not found")
}

type mockLogger struct{}

func (m *mockLogger) Error(msg string, keysAndValues ...any) {}
func (m *mockLogger) Info(msg string, keysAndValues ...any)  {}

func TestCache_ListVersions(t *testing.T) {
	store := &mockKeyValueStore{data: make(map[string][]string)}
	repo := &mockCacheRepository{versions: map[string][]string{
		"owner/repo/module": {"v1.0.0", "v1.1.0"},
	}}
	cache := NewCache(repo, store, nil, nil, false)

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
	cache := NewCache(repo, store, files, logger, false)

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

func TestCache_ListVersions_CacheHit(t *testing.T) {
	wantErr := errors.New("forbidden")
	tests := map[string]struct {
		repoHeadErr  error
		authDisabled bool
		wantErr      error
		wantVersions []string
	}{
		"RepoHeadSucceeds": {
			repoHeadErr:  nil,
			wantVersions: []string{"v1.0.0", "v1.1.0"},
		},
		"RepoHeadFails": {
			repoHeadErr: wantErr,
			wantErr:     wantErr,
		},
		"AuthDisabled": {
			// RepoHead would fail, but auth is disabled so it must not be consulted.
			repoHeadErr:  wantErr,
			authDisabled: true,
			wantVersions: []string{"v1.0.0", "v1.1.0"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			store := &mockKeyValueStore{data: map[string][]string{
				"owner-repo-module": {"v1.0.0", "v1.1.0"},
			}}
			repo := &mockCacheRepository{repoHeadSet: true, repoHeadErr: tc.repoHeadErr}
			cache := NewCache(repo, store, nil, &mockLogger{}, tc.authDisabled)

			versions, err := cache.ListVersions(context.Background(), "owner", "repo", "module")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
			if len(versions) != len(tc.wantVersions) {
				t.Fatalf("expected %d versions, got %d", len(tc.wantVersions), len(versions))
			}
			for i, v := range versions {
				if v != tc.wantVersions[i] {
					t.Errorf("expected version %q, got %q", tc.wantVersions[i], v)
				}
			}
		})
	}
}

func TestCache_ProxyDownload_CacheHit(t *testing.T) {
	const cached = "cached tarball content"
	wantErr := errors.New("forbidden")
	tests := map[string]struct {
		repoHeadErr error
		wantErr     error
		wantContent string
	}{
		"RepoHeadSucceeds": {
			repoHeadErr: nil,
			wantContent: cached,
		},
		"RepoHeadFails": {
			repoHeadErr: wantErr,
			wantErr:     wantErr,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			store := &mockKeyValueStore{data: make(map[string][]string)}
			files := &mockFileStorage{files: map[string][]byte{
				"owner-repo-module-v1.0.0.tar.gz": []byte(cached),
			}}
			repo := &mockCacheRepository{repoHeadSet: true, repoHeadErr: tc.repoHeadErr}
			cache := NewCache(repo, store, files, &mockLogger{}, false)

			var buf bytes.Buffer
			err := cache.ProxyDownload(context.Background(), "owner", "repo", "module", "v1.0.0", &buf)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
			if buf.String() != tc.wantContent {
				t.Errorf("expected content %q, got %q", tc.wantContent, buf.String())
			}
		})
	}
}
