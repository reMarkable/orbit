package modules

import (
	"fmt"
	"log/slog"
	"net/http"
)

type (
	Metrics struct {
		requestCount  map[string]int
		downloadCount map[string]int
	}
	MetricsHandler struct {
		logger  Logger
		metrics Metrics
	}
)

type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
	MetricTypeUntyped   MetricType = "untyped"
)

func NewMetricsHandler(log Logger) (*MetricsHandler, error) {
	m := Metrics{
		requestCount:  make(map[string]int),
		downloadCount: make(map[string]int),
	}
	return &MetricsHandler{logger: log, metrics: m}, nil
}

func (h *MetricsHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	h.writeMeta(w, "Request Count", "Total number of requests received", "request_count")
	for endpoint, count := range h.metrics.requestCount {
		h.writeMetrics(w, "request_count", map[string]string{"endpoint": endpoint}, count)
	}
	h.writeMeta(w, "Download Count", "Total number of downloads", "download_count")
	for module, count := range h.metrics.downloadCount {
		h.writeMetrics(w, "download_count", map[string]string{"module": module}, count)
	}
}

func (h *MetricsHandler) writeMeta(w http.ResponseWriter, metricType MetricType, help string, metric string) {
	meta := fmt.Sprintf("# HELP %s %s\n# TYPE %s %s\n", metric, help, metric, metricType)
	w.Write([]byte(meta))
}

func (h *MetricsHandler) writeMetrics(w http.ResponseWriter, metric string, labels map[string]string, value int) {
	meta := fmt.Sprintf("%s{", metric)
	for k, v := range labels {
		meta += fmt.Sprintf("%s=\"%s\",", k, v)
	}
	meta = meta[:len(meta)-1] + fmt.Sprintf("} %d\n", value)
	_, err := w.Write([]byte(meta))
	if err != nil {
		slog.Error("write error", "err", err)
	}
}

func (h *MetricsHandler) IncrementRequestCount(endpoint string) {
	h.metrics.requestCount[endpoint]++
}

func (h *MetricsHandler) IncrementDownloadCount(namespace, name string) {
	h.metrics.downloadCount[namespace+"/"+name]++
}

func numByte(i int) []byte {
	return fmt.Appendf(nil, "%d", i)
}
