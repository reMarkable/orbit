// Copyright 2023 Henrik Hedlund. All rights reserved.
// Use of this source code is governed by the GNU Affero
// GPL license that can be found in the LICENSE file.

package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var timeoutWiggleRoom = 300 * time.Millisecond

type Logger interface {
	Error(msg string, args ...any)
	Info(msg string, args ...any)
}

type Config struct {
	Host    string `envconfig:"HOST" default:""`
	Port    int    `envconfig:"PORT" default:"8080"`
	Timeout struct {
		Handler    time.Duration `envconfig:"TIMEOUT_HANDLER" default:"10s"`
		Idle       time.Duration `envconfig:"TIMEOUT_IDLE"`
		Read       time.Duration `envconfig:"TIMEOUT_READ"`
		ReadHeader time.Duration `envconfig:"TIMEOUT_READ_HEADER" default:"2s"`
		Shutdown   time.Duration `envconfig:"TIMEOUT_SHUTDOWN" default:"5s"`
		Write      time.Duration `envconfig:"TIMEOUT_WRITE"`
	}
	TLS struct {
		Enabled  bool   `envconfig:"TLS_ENABLED"`
		CertFile string `envconfig:"TLS_CERT_FILE"`
		KeyFile  string `envconfig:"TLS_KEY_FILE"`
	}
	Metrics struct {
		Enabled bool `envconfig:"METRICS_ENABLED" default:"false"`
		Port    int  `envconfig:"METRICS_PORT" default:"9090"`
	}
}

func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func (c *Config) MetricsListenAddr() string {
	// Metrics listener typically binds to the same host as the main server
	return fmt.Sprintf("%s:%d", c.Host, c.Metrics.Port)
}

func (c *Config) ReadTimeout() time.Duration {
	if c.Timeout.Read > 0 {
		return c.Timeout.Read
	}
	if c.Timeout.Handler > 0 {
		return c.Timeout.Handler + c.Timeout.ReadHeader + timeoutWiggleRoom
	}
	return 0
}

func (c *Config) WriteTimeout() time.Duration {
	if c.Timeout.Write > 0 {
		return c.Timeout.Write
	}
	if c.Timeout.Handler > 0 {
		return c.Timeout.Handler + timeoutWiggleRoom
	}
	return 0
}

func Start(cfg Config, log Logger, h http.Handler, listening ...chan net.Addr) error {
	if cfg.Timeout.Handler > 0 {
		h = http.TimeoutHandler(h, cfg.Timeout.Handler, "request timeout")
	}

	ln, err := net.Listen("tcp", cfg.ListenAddr())
	var metricsLn net.Listener
	if err != nil {
		log.Error("listen error", "err", err)
		return err
	}
	log.Info("server listening", "addr", ln.Addr().String())

	if len(listening) > 0 {
		listening[0] <- ln.Addr()
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           h,
		IdleTimeout:       cfg.Timeout.Idle,
		ReadTimeout:       cfg.ReadTimeout(),
		ReadHeaderTimeout: cfg.Timeout.ReadHeader,
		WriteTimeout:      cfg.WriteTimeout(),
	}
	var metricsSrv *http.Server
	if cfg.Metrics.Enabled {
		metricsLn, err = net.Listen("tcp", cfg.MetricsListenAddr())
		if err != nil {
			ln.Close() // Close the main listener if metrics listener fails
			log.Error("metrics listen error", "err", err)
			return err
		}
		log.Info("metrics server listening", "addr", metricsLn.Addr().String())
		metricsSrv = &http.Server{
			Addr:              cfg.MetricsListenAddr(),
			Handler:           http.DefaultServeMux,
			IdleTimeout:       cfg.Timeout.Idle,       // Reuse main timeouts
			ReadTimeout:       cfg.ReadTimeout(),      // Reuse main timeouts
			ReadHeaderTimeout: cfg.Timeout.ReadHeader, // Reuse main timeouts
			WriteTimeout:      cfg.WriteTimeout(),     // Reuse main timeouts
		}
	}

	errs := make(chan error)
	if cfg.TLS.Enabled {
		go serveTLS(srv, ln, cfg.TLS.CertFile, cfg.TLS.KeyFile, errs)
	} else {
		go serve(srv, ln, errs)
	}
	if metricsSrv != nil {
		go serve(metricsSrv, metricsLn, errs)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-done:
		log.Info("server shutting down")
	case err := <-errs:
		log.Error("serve error", "err", err)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout.Shutdown)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown error", "err", err)
		return err
	}
	if metricsSrv != nil {
		if err := metricsSrv.Shutdown(ctx); err != nil {
			log.Error("metrics shutdown error", "err", err)
		}
	}
	return nil
}

func serve(s *http.Server, ln net.Listener, errs chan error) {
	if err := s.Serve(ln); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}
}

func serveTLS(s *http.Server, ln net.Listener, certFile, keyFile string, errs chan error) {
	if err := s.ServeTLS(ln, certFile, keyFile); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}
}
