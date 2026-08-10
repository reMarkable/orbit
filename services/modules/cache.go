// Copyright 2023 Henrik Hedlund. All rights reserved.
// Use of this source code is governed by the GNU Affero
// GPL license that can be found in the LICENSE file.

package modules

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type KeyValueStore interface {
	Get(key string) ([]string, bool)
	Set(key string, value []string, d ...time.Duration)
}

type FileStorage interface {
	Open(filename string) (io.ReadCloser, error)
	Create(filename string) (CacheWriter, error)
}

// CacheWriter is a handle to a cache entry that is being written. The written
// content only becomes visible to Open after a successful Commit; closing
// without committing discards it, so readers never observe partial writes.
type CacheWriter interface {
	io.Writer
	// Commit atomically publishes the written content to readers.
	Commit() error
	// Close releases resources, discarding any uncommitted content.
	io.Closer
}

func NewCache(r Repository, s KeyValueStore, f FileStorage, l Logger, authDisabled bool) *Cache {
	return &Cache{
		files:        f,
		log:          l,
		repo:         r,
		store:        s,
		authDisabled: authDisabled,
		locks:        make(map[string]*keyLock),
	}
}

type Cache struct {
	files        FileStorage
	log          Logger
	repo         Repository
	store        KeyValueStore
	authDisabled bool

	// mu guards locks, the set of per-filename locks used to coalesce
	// concurrent downloads of the same artifact.
	mu    sync.Mutex
	locks map[string]*keyLock
}

// RepoHead is used to check if we can access the repository in a cheap way.
func (c *Cache) RepoHead(ctx context.Context, owner, repo string) error {
	if c.authDisabled {
		return nil
	}

	return c.repo.RepoHead(ctx, owner, repo)
}

func (c *Cache) ListVersions(ctx context.Context, owner, repo, module string) ([]string, error) {
	key := fmt.Sprintf("%s-%s-%s", owner, repo, module)
	if v, ok := c.store.Get(key); ok {

		err := c.RepoHead(ctx, owner, repo)
		if err != nil {
			return nil, err
		}

		return v, nil
	}

	v, err := c.repo.ListVersions(ctx, owner, repo, module)
	if err != nil {
		return nil, err
	}

	c.store.Set(key, v)
	return v, nil
}

func (c *Cache) ProxyDownload(ctx context.Context, owner, repo, module, version string, w io.Writer) error {
	filename := fmt.Sprintf("%s-%s-%s-%s.tar.gz", owner, repo, module, version)

	// Verify repository access before writing any bytes, so we never leak
	// cached content to callers who do not have access to the repository.
	if err := c.RepoHead(ctx, owner, repo); err != nil {
		return err
	}

	// Fast path: serve directly from the cache when the file already exists.
	if served, err := c.serveCached(filename, w); served {
		return err
	}

	// Cache miss: take a per-filename lock so that only one goroutine downloads
	// and writes a given artifact at a time. Concurrent callers block here and
	// then serve the freshly cached file below, avoiding duplicate downloads
	// and interleaved writes to the same file.
	unlock := c.lockKey(filename)
	defer unlock()

	// Another goroutine may have populated the cache while we waited.
	if served, err := c.serveCached(filename, w); served {
		return err
	}

	if err := c.cacheDownload(ctx, filename, owner, repo, module, version); err != nil {
		return err
	}

	if served, err := c.serveCached(filename, w); served {
		return err
	}

	// Caching is unavailable (e.g. we failed to create the file); fall back to
	// proxying the download directly from the repository.
	return c.repo.ProxyDownload(ctx, owner, repo, module, version, w)
}

// serveCached copies a cached file to w when it exists. It returns true if the
// file was found (and thus handled), along with any error from copying.
// Callers must have verified repository access before calling.
func (c *Cache) serveCached(filename string, w io.Writer) (bool, error) {
	r, err := c.files.Open(filename)
	if err != nil {
		return false, nil
	}
	defer closer(r, c.log, "failed to close read cached file")

	if _, err := io.Copy(w, r); err != nil {
		c.log.Error("failed to copy cached file", "err", err)
		return true, err
	}

	return true, nil
}

// cacheDownload downloads the artifact from the repository into the cache. The
// content is written to a temporary file and only published on success, so a
// failed or partial download never becomes visible to readers. A failure to
// create the cache file is not fatal: it is logged and nil is returned so the
// caller can fall back to a direct proxy download.
func (c *Cache) cacheDownload(ctx context.Context, filename, owner, repo, module, version string) error {
	cw, err := c.files.Create(filename)
	if err != nil {
		c.log.Error("failed to create cached file", "err", err)
		return nil
	}
	defer closer(cw, c.log, "failed to close created cached file")

	if err := c.repo.ProxyDownload(ctx, owner, repo, module, version, cw); err != nil {
		return err
	}
	return cw.Commit()
}

// keyLock is a mutex with a reference count, allowing unused entries to be
// removed from the Cache lock map once no goroutine holds or waits on them.
type keyLock struct {
	mu  sync.Mutex
	ref int
}

// lockKey acquires the lock associated with key, creating it if necessary, and
// returns a function that releases it and cleans up the entry when idle.
func (c *Cache) lockKey(key string) func() {
	c.mu.Lock()
	kl, ok := c.locks[key]
	if !ok {
		kl = &keyLock{}
		c.locks[key] = kl
	}
	kl.ref++
	c.mu.Unlock()

	kl.mu.Lock()
	return func() {
		kl.mu.Unlock()

		c.mu.Lock()
		kl.ref--
		if kl.ref == 0 {
			delete(c.locks, key)
		}
		c.mu.Unlock()
	}
}

// StoreInPath implements the FileStorage interface by storing files locally on
// the file-system at the specified path. It's a bare minimum implementation,
// and doesn't create any folders.
type StoreInPath string

func (s StoreInPath) Open(filename string) (io.ReadCloser, error) {
	return os.Open(s.path(filename))
}

func (s StoreInPath) Create(filename string) (CacheWriter, error) {
	final := s.path(filename)
	f, err := os.CreateTemp(string(s), filepath.Base(filename)+".tmp-*")
	if err != nil {
		return nil, err
	}

	return &atomicFile{file: f, finalPath: final}, nil
}

func (s StoreInPath) path(filename string) string {
	return fmt.Sprintf("%s/%s", s, filename)
}

// atomicFile writes to a temporary file and only moves it into its final
// location on Commit, so concurrent readers never observe a partial file.
// Closing before Commit discards the temporary file.
type atomicFile struct {
	file      *os.File
	finalPath string
	committed bool
}

func (a *atomicFile) Write(p []byte) (int, error) {
	return a.file.Write(p)
}

func (a *atomicFile) Commit() error {
	if err := a.file.Sync(); err != nil {
		return err
	}

	if err := a.file.Close(); err != nil {
		return err
	}

	if err := os.Rename(a.file.Name(), a.finalPath); err != nil {
		return err
	}

	a.committed = true
	return nil
}

func (a *atomicFile) Close() error {
	if a.committed {
		return nil
	}

	// Discard the temporary file when it was never committed.
	_ = a.file.Close()
	return os.Remove(a.file.Name())
}

// closer simply closes the closer and logs any errors.
func closer(c io.Closer, log Logger, msg string) {
	if err := c.Close(); err != nil {
		log.Error(msg, "err", err)
	}
}
