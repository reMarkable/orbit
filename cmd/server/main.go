// Copyright 2023 Henrik Hedlund. All rights reserved.
// Use of this source code is governed by the GNU Affero
// GPL license that can be found in the LICENSE file.

package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/reMarkable/orbit/pkg/auth"
	"github.com/reMarkable/orbit/pkg/envconfig"
	"github.com/reMarkable/orbit/pkg/github"
	"github.com/reMarkable/orbit/pkg/mcache"
	"github.com/reMarkable/orbit/pkg/router"
	"github.com/reMarkable/orbit/pkg/server"
	"github.com/reMarkable/orbit/services/modules"
)

type config struct {
	Cache struct {
		Enabled      bool          `envconfig:"ENABLED"`
		AuthDisabled bool          `envconfig:"AUTH_DISABLED"`
		Path         string        `envconfig:"PATH" default:"/tmp"`
		Expiration   time.Duration `envconfig:"EXPIRATION" default:"1m"`
	} `envconfig:"CACHE_"`
	Github  github.Config  `envconfig:"GITHUB_"`
	Modules modules.Config `envconfig:"MODULES_"`
	Server  server.Config
}

func main() {
	var cfg config
	envconfig.MustProcess(&cfg)

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	var cache modules.KeyValueStore
	if cfg.Cache.Enabled {
		c := mcache.New[string, []string](cfg.Cache.Expiration)
		if cfg.Cache.Expiration > 0 {
			// Start a cleanup loop to remove expired items from the cache periodically.
			stop := mcache.StartCleanupLoop(c, cfg.Cache.Expiration)
			defer stop()
		}
		cache = c
	}

	var repo modules.Repository
	repo = github.New(cfg.Github, &http.Client{
		Timeout: 5 * time.Second,
	}, cache)

	if cfg.Cache.Enabled {
		log.Info("enabling cache", "path", cfg.Cache.Path, "expiration", cfg.Cache.Expiration, "authDisabled", cfg.Cache.AuthDisabled)
		if cfg.Cache.AuthDisabled {
			log.Warn("auth is disabled for cached request. This is dangerous when used with private repos, or in environments with no other access control mechanisms in place")
		}
		repo = modules.NewCache(
			repo,
			cache,
			modules.StoreInPath(cfg.Cache.Path),
			log,
			cfg.Cache.AuthDisabled,
		)
	}

	mh, err := modules.NewMetricsHandler(log)
	if err != nil {
		panic(err)
	}
	h, err := modules.NewHTTP(cfg.Modules, log, repo, mh)
	if err != nil {
		panic(err)
	}

	r := router.New()
	r.Use(auth.TokenMiddleware)

	r.Get("/v1/modules/:namespace/:name/:system/versions", h.ListVersions)
	r.Get("/v1/modules/:namespace/:name/:system/:version/download", h.DownloadURL)
	r.Get("/v1/modules/:namespace/:name/:system/:version/proxy", h.ProxyDownload)
	r.Get("/.well-known/terraform.json", discovery)

	http.DefaultServeMux.HandleFunc("GET /metrics", mh.Metrics)

	if err := server.Start(cfg.Server, log, r); err != nil {
		panic(err)
	}
}

func discovery(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	if _, err := w.Write([]byte(`{"modules.v1":"/v1/modules"}`)); err != nil {
		slog.Error("failed to write response", "error", err)
	}
}
