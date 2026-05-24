package monitoring

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

func PrometheusHandler(collector *MetricsCollector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		metrics := collector.GetAllMetrics()
		if len(metrics) == 0 {
			w.WriteHeader(http.StatusOK)
			return
		}

		byName := make(map[string][]Metric)
		for _, m := range metrics {
			byName[m.Name] = append(byName[m.Name], m)
		}

		var b strings.Builder
		for name, group := range byName {
			if len(group) == 0 {
				continue
			}
			if group[0].Type == MetricTypeHistogram {
				continue
		}
		sanitized := sanitizeMetricName(name)
		fmt.Fprintf(&b, "# HELP %s atlas-go metric %s\n", sanitized, name)
		fmt.Fprintf(&b, "# TYPE %s %s\n", sanitized, string(group[0].Type))
		for _, m := range group {
			b.WriteString(formatMetricLine(sanitized, m))
		}
			b.WriteString("\n")
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(b.String()))
	}
}

func sanitizeMetricName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == ':':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func formatMetricLine(name string, m Metric) string {
	var b strings.Builder
	b.WriteString(name)
	if len(m.Labels) > 0 {
		b.WriteString("{")
		keys := make([]string, 0, len(m.Labels))
		for k := range m.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, "%s=\"%s\"", sanitizeLabelName(k), escapeLabelValue(m.Labels[k]))
		}
		b.WriteString("}")
	}
	fmt.Fprintf(&b, " %.6f\n", m.Value)
	return b.String()
}

func sanitizeLabelName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func escapeLabelValue(v string) string {
	v = strings.ReplaceAll(v, "\\", "\\\\")
	v = strings.ReplaceAll(v, "\"", "\\\"")
	v = strings.ReplaceAll(v, "\n", "\\n")
	return v
}
