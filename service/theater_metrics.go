package service

import (
	"sort"
	"strings"
	"sync"
)

type TheaterMetricSnapshot struct {
	Counters map[string]float64 `json:"counters"`
}

var theaterMetrics = struct {
	sync.RWMutex
	counters map[string]float64
}{counters: map[string]float64{}}

var theaterMetricAllowedLabels = map[string]struct{}{
	"type": {}, "outcome": {}, "permission": {}, "status": {}, "mime": {},
}

func RecordTheaterMetric(name string, labels map[string]string, value float64) {
	name = strings.TrimSpace(name)
	if name == "" || value == 0 {
		return
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		if _, ok := theaterMetricAllowedLabels[key]; ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var builder strings.Builder
	builder.WriteString(name)
	for _, key := range keys {
		value := sanitizeTheaterMetricLabel(labels[key])
		if value == "" {
			continue
		}
		builder.WriteByte('|')
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(value)
	}
	theaterMetrics.Lock()
	theaterMetrics.counters[builder.String()] += value
	theaterMetrics.Unlock()
}

func sanitizeTheaterMetricLabel(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 64 {
		value = value[:64]
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("._-/", char) {
			continue
		}
		return "other"
	}
	return value
}

func TheaterMetricsSnapshot() TheaterMetricSnapshot {
	theaterMetrics.RLock()
	defer theaterMetrics.RUnlock()
	counters := make(map[string]float64, len(theaterMetrics.counters))
	for key, value := range theaterMetrics.counters {
		counters[key] = value
	}
	return TheaterMetricSnapshot{Counters: counters}
}

func ResetTheaterMetricsForTest() {
	theaterMetrics.Lock()
	theaterMetrics.counters = map[string]float64{}
	theaterMetrics.Unlock()
}
