package modules

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type MockLogger struct{}

func (m MockLogger) Error(msg string, args ...any) {}
func (m MockLogger) Info(msg string, args ...any)  {}

func TestNewMetricsHandler(t *testing.T) {
	logger := MockLogger{}
	handler, err := NewMetricsHandler(logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler == nil {
		t.Fatal("expected non-nil MetricsHandler")
	}
}

func TestMetricsHandler_Metrics(t *testing.T) {
	logger := MockLogger{}
	handler, _ := NewMetricsHandler(logger)

	handler.IncrementRequestCount("/test")
	handler.IncrementDownloadCount("namespace", "module")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.Metrics(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !containsMetric(body, "request_count", "endpoint", "/test", 1) {
		t.Error("expected request_count metric for /test")
	}
	if !containsMetric(body, "download_count", "module", "namespace/module", 1) {
		t.Error("expected download_count metric for namespace/module")
	}
}

func containsMetric(body, metric, labelKey, labelValue string, value int) bool {
	expected := metric + "{" + labelKey + "=\"" + labelValue + "\"} " + fmt.Sprintf("%d", value)
	return strings.Contains(body, expected)
}

func TestMetricsHandler_Metrics_Type(t *testing.T) {
	logger := MockLogger{}
	handler, _ := NewMetricsHandler(logger)

	handler.IncrementRequestCount("/test")
	handler.IncrementDownloadCount("namespace", "module")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.Metrics(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "# TYPE request_count counter") {
		t.Error("expected metric type 'counter' for request_count")
	}
	if !strings.Contains(body, "# TYPE download_count counter") {
		t.Error("expected metric type 'counter' for download_count")
	}
}
