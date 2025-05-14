package modules

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
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
	handler.IncrementDownloadCount("namespace", "module", "0.1")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.Metrics(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	requestCountExpected := formatMetric("request_count", map[string]string{"endpoint": "/test"}, 1)
	if !strings.Contains(body, requestCountExpected) {
		t.Errorf("expected '%s' for request_count metric for /test, got: %s", requestCountExpected, body)
	}
	downloadCountExpected := formatMetric("download_count", map[string]string{"namespace": "namespace", "module": "module", "version": "0.1"}, 1)
	if !strings.Contains(body, downloadCountExpected) {
		t.Errorf("expected %s for namespace/module in response body, got: %s", downloadCountExpected, body)
	}
}

func formatMetric(metric string, labels map[string]string, value int) string {
	var labelParts []string
	for _, k := range slices.Sorted(maps.Keys(labels)) {
		v := labels[k]
		labelParts = append(labelParts, fmt.Sprintf("%s=\"%s\"", k, v))
	}
	labelStr := strings.Join(labelParts, ",")
	return metric + "{" + labelStr + "} " + fmt.Sprintf("%d", value)
}

func TestMetricsHandler_Metrics_Type(t *testing.T) {
	logger := MockLogger{}
	handler, _ := NewMetricsHandler(logger)

	handler.IncrementRequestCount("/test")
	handler.IncrementDownloadCount("namespace", "module", "0.1")

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
