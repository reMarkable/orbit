package modules

import (
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
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

type MetricType int

const (
	MetricTypeCounter MetricType = iota
	MetricTypeGauge
	MetricTypeHistogram
	MetricTypeSummary
	MetricTypeUntyped
)

var metricTypeName = map[MetricType]string{
	MetricTypeCounter:   "counter",
	MetricTypeGauge:     "gauge",
	MetricTypeHistogram: "histogram",
	MetricTypeSummary:   "summary",
	MetricTypeUntyped:   "untyped",
}

func (mt MetricType) String() string {
	return metricTypeName[mt]
}

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
	h.writeMeta(w, MetricTypeCounter, "Total number of requests received", "request_count")
	for endpoint, count := range h.metrics.requestCount {
		h.writeMetrics(w, "request_count", map[string]string{"endpoint": endpoint}, count)
	}
	h.writeMeta(w, MetricTypeCounter, "Total number of downloads", "download_count")
	for module, count := range h.metrics.downloadCount {
		meta := strings.Split(module, "/")
		h.writeMetrics(w, "download_count", map[string]string{"module": meta[1], "namespace": meta[0], "version": meta[2]}, count)
	}
}

func (h *MetricsHandler) writeMeta(w http.ResponseWriter, metricType MetricType, help string, metric string) {
	meta := fmt.Sprintf("# HELP %s %s\n# TYPE %s %s\n", metric, help, metric, metricType)
	w.Write([]byte(meta))
}

func (h *MetricsHandler) writeMetrics(w http.ResponseWriter, metric string, labels map[string]string, value int) {
	meta := fmt.Sprintf("%s{", metric)
	for _, k := range slices.Sorted(maps.Keys(labels)) {
		meta += fmt.Sprintf("%s=\"%s\",", k, labels[k])
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

func (h *MetricsHandler) IncrementDownloadCount(namespace string, name string, version string) {
	h.metrics.downloadCount[namespace+"/"+name+"/"+version]++
}
