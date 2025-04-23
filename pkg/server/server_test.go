package server

import (
	"net"
	"net/http"
	"testing"
	"time"
)

type mockLogger struct {
	errors []string
	infos  []string
}

func (m *mockLogger) Error(msg string, args ...any) {
	m.errors = append(m.errors, msg)
}

func (m *mockLogger) Info(msg string, args ...any) {
	m.infos = append(m.infos, msg)
}

func TestStartServer(t *testing.T) {
	cfg := Config{
		Host:    "127.0.0.1",
		Port:    8081,
		Timeout: Config{}.Timeout,
	}

	logger := &mockLogger{}
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	listening := make(chan net.Addr, 1)
	go func() {
		if err := Start(cfg, logger, handler, listening); err != nil {
			t.Errorf("server failed to start: %v", err)
		}
	}()

	select {
	case addr := <-listening:
		if addr == nil {
			t.Error("server did not start listening")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for server to start")
	}
}
