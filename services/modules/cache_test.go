package modules

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
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
	mu        sync.Mutex
	files     map[string][]byte
	createErr error
	created   atomic.Int32
}

func (m *mockFileStorage) Open(filename string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[filename]
	if !ok {
		return nil, errors.New("file not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *mockFileStorage) Create(filename string) (CacheWriter, error) {
	m.created.Add(1)
	if m.createErr != nil {
		return nil, m.createErr
	}
	return &mockCacheWriter{fs: m, name: filename}, nil
}

func (m *mockFileStorage) get(filename string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.files[filename]
	return v, ok
}

// mockCacheWriter mimics atomic writes: content is only published to the store
// on Commit, so an uncommitted (partial) write is never visible to Open.
type mockCacheWriter struct {
	bytes.Buffer
	fs   *mockFileStorage
	name string
}

func (w *mockCacheWriter) Commit() error {
	w.fs.mu.Lock()
	defer w.fs.mu.Unlock()
	w.fs.files[w.name] = append([]byte(nil), w.Buffer.Bytes()...)
	return nil
}

func (w *mockCacheWriter) Close() error { return nil }

type mockCacheRepository struct {
	versions    map[string][]string
	repoHeadSet bool
	repoHeadErr error

	proxyFunc  func(w io.Writer) error
	proxyCalls atomic.Int32
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
	m.proxyCalls.Add(1)
	if m.proxyFunc != nil {
		return m.proxyFunc(w)
	}
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
	repo := &mockCacheRepository{repoHeadSet: true}
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

// A cache miss must download once, publish the file to the cache via Commit,
// and serve the same content to the caller.
func TestCache_ProxyDownload_CachesAfterMiss(t *testing.T) {
	const filename = "owner-repo-module-v1.0.0.tar.gz"
	store := &mockKeyValueStore{data: make(map[string][]string)}
	files := &mockFileStorage{files: make(map[string][]byte)}
	repo := &mockCacheRepository{repoHeadSet: true}
	cache := NewCache(repo, store, files, &mockLogger{}, false)

	var buf bytes.Buffer
	if err := cache.ProxyDownload(context.Background(), "owner", "repo", "module", "v1.0.0", &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := buf.String(); got != "fake tarball content" {
		t.Errorf("expected content %q, got %q", "fake tarball content", got)
	}
	if got := files.created.Load(); got != 1 {
		t.Errorf("expected 1 cache file created, got %d", got)
	}
	if data, ok := files.get(filename); !ok {
		t.Error("expected file to be committed to the cache")
	} else if string(data) != "fake tarball content" {
		t.Errorf("expected cached content %q, got %q", "fake tarball content", string(data))
	}
}

// A failed (partial) download must not be committed to the cache, and no
// partial content must reach the caller.
func TestCache_ProxyDownload_DownloadFailureNotCached(t *testing.T) {
	const filename = "owner-repo-module-v1.0.0.tar.gz"
	wantErr := errors.New("boom")
	store := &mockKeyValueStore{data: make(map[string][]string)}
	files := &mockFileStorage{files: make(map[string][]byte)}
	repo := &mockCacheRepository{
		repoHeadSet: true,
		proxyFunc: func(w io.Writer) error {
			_, _ = w.Write([]byte("partial"))
			return wantErr
		},
	}
	cache := NewCache(repo, store, files, &mockLogger{}, false)

	var buf bytes.Buffer
	err := cache.ProxyDownload(context.Background(), "owner", "repo", "module", "v1.0.0", &buf)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no content served, got %q", buf.String())
	}
	if _, ok := files.get(filename); ok {
		t.Error("expected no file to be committed after a failed download")
	}
}

// When the cache file cannot be created, the download must be proxied directly
// from the repository without caching.
func TestCache_ProxyDownload_CreateFailureFallsBack(t *testing.T) {
	const filename = "owner-repo-module-v1.0.0.tar.gz"
	store := &mockKeyValueStore{data: make(map[string][]string)}
	files := &mockFileStorage{files: make(map[string][]byte), createErr: errors.New("disk full")}
	repo := &mockCacheRepository{repoHeadSet: true}
	cache := NewCache(repo, store, files, &mockLogger{}, false)

	var buf bytes.Buffer
	if err := cache.ProxyDownload(context.Background(), "owner", "repo", "module", "v1.0.0", &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := buf.String(); got != "fake tarball content" {
		t.Errorf("expected content %q, got %q", "fake tarball content", got)
	}
	if _, ok := files.get(filename); ok {
		t.Error("expected no file to be committed when create fails")
	}
	if got := repo.proxyCalls.Load(); got != 1 {
		t.Errorf("expected 1 proxy download, got %d", got)
	}
}

// Concurrent requests for the same artifact must be coalesced: the repository
// is downloaded exactly once and every caller receives the full content.
func TestCache_ProxyDownload_ConcurrentCoalesced(t *testing.T) {
	const goroutines = 20
	store := &mockKeyValueStore{data: make(map[string][]string)}
	files := &mockFileStorage{files: make(map[string][]byte)}
	repo := &mockCacheRepository{
		repoHeadSet: true,
		proxyFunc: func(w io.Writer) error {
			// Hold the lock briefly so callers genuinely overlap.
			time.Sleep(10 * time.Millisecond)
			_, err := w.Write([]byte("fake tarball content"))
			return err
		},
	}
	cache := NewCache(repo, store, files, &mockLogger{}, false)

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	bufs := make([]bytes.Buffer, goroutines)
	errs := make([]error, goroutines)
	for i := range goroutines {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			start.Wait()
			errs[i] = cache.ProxyDownload(context.Background(), "owner", "repo", "module", "v1.0.0", &bufs[i])
		}(i)
	}
	start.Done()
	done.Wait()

	for i := range goroutines {
		if errs[i] != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, errs[i])
		}
		if got := bufs[i].String(); got != "fake tarball content" {
			t.Errorf("goroutine %d: expected content %q, got %q", i, "fake tarball content", got)
		}
	}
	if got := repo.proxyCalls.Load(); got != 1 {
		t.Errorf("expected exactly 1 download, got %d", got)
	}
	if got := files.created.Load(); got != 1 {
		t.Errorf("expected exactly 1 cache file created, got %d", got)
	}
}

// StoreInPath must only make content visible after Commit, so readers never
// observe a partial file.
func TestStoreInPath_AtomicCommit(t *testing.T) {
	dir := t.TempDir()
	store := StoreInPath(dir)

	cw, err := store.Create("module.tar.gz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := cw.Write([]byte("hello world")); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}

	// Before Commit the file must not be readable at its final location.
	if _, err := store.Open("module.tar.gz"); err == nil {
		t.Error("expected Open to fail before Commit")
	}

	if err := cw.Commit(); err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if err := cw.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}

	r, err := store.Open("module.tar.gz")
	if err != nil {
		t.Fatalf("expected Open to succeed after Commit: %v", err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", string(data))
	}
}

// Closing without committing must discard the content and leave no temp files.
func TestStoreInPath_CloseDiscardsUncommitted(t *testing.T) {
	dir := t.TempDir()
	store := StoreInPath(dir)

	cw, err := store.Create("module.tar.gz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := cw.Write([]byte("partial")); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if err := cw.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}

	if _, err := store.Open("module.tar.gz"); err == nil {
		t.Error("expected Open to fail for uncommitted file")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("unexpected error reading dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no leftover files, got %d", len(entries))
	}
}
