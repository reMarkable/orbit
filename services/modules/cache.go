// Copyright 2023 Henrik Hedlund. All rights reserved.
// Use of this source code is governed by the GNU Affero
// GPL license that can be found in the LICENSE file.

package modules

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"
)

type KeyValueStore interface {
	Get(key string) ([]string, bool)
	Set(key string, value []string, d ...time.Duration)
}

type FileStorage interface {
	Open(filename string) (io.ReadCloser, error)
	Create(filename string) (io.WriteCloser, error)
}

func NewCache(r Repository, s KeyValueStore, f FileStorage, l Logger, authDisabled bool) *Cache {
	return &Cache{f, l, r, s, authDisabled}
}

type Cache struct {
	files        FileStorage
	log          Logger
	repo         Repository
	store        KeyValueStore
	authDisabled bool
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
	if r, err := c.files.Open(filename); err != nil {
		// If we just fail to open the cached file, we'll just log the error and
		// then re-download it from the repository as usual.
		c.log.Error("failed to open cached file", "err", err)
	} else {
		defer func() {
			if err := r.Close(); err != nil {
				slog.Error("failed to close cached file", "err", err)
			}
		}()

		// Verify repository access before writing any cached content to the
		// response, otherwise we may leak the cached file to callers who do
		// not have access to the repository.
		if err := c.RepoHead(ctx, owner, repo); err != nil {
			return err
		}

		if _, err := io.Copy(w, r); err != nil {
			c.log.Error("failed to copy cached file", "err", err)
			return err
		}

		// At this point we have copied the cached file, so we are done.
		return nil
	}

	if cw, err := c.files.Create(filename); err != nil {
		// If we fail to create a cache file, we'll just proxy download directly
		// from the repository without caching.
		c.log.Error("failed to create cached file", "err", err)
	} else {
		defer func() {
			if err := cw.Close(); err != nil {
				slog.Error("failed to close cached file", "err", err)
			}
		}()
		w = io.MultiWriter(w, cw)
	}
	return c.repo.ProxyDownload(ctx, owner, repo, module, version, w)
}

// StoreInPath implements the FileStorage interface by storing files locally on
// the file-system at the specified path. It's a bare minimum implementation,
// and doesn't create any folders.
type StoreInPath string

func (s StoreInPath) Open(filename string) (io.ReadCloser, error) {
	return os.Open(s.path(filename))
}

func (s StoreInPath) Create(filename string) (io.WriteCloser, error) {
	return os.Create(s.path(filename))
}

func (s StoreInPath) path(filename string) string {
	return fmt.Sprintf("%s/%s", s, filename)
}
