package promtextfile

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/atamostec/file-collector/internal/config"
	"github.com/atamostec/file-collector/internal/tabular"
)

func ExportInputMetrics(input config.Input, row tabular.TypedRow) error {
	cfg := input.PrometheusTextfile
	if !cfg.Enabled {
		return nil
	}

	lines := make([]string, 0, len(cfg.Metrics)*6)
	seenHelp := make(map[string]struct{}, len(cfg.Metrics)*2)
	seenType := make(map[string]struct{}, len(cfg.Metrics)*2)

	for _, metric := range cfg.Metrics {
		metricType := strings.TrimSpace(metric.Type)
		if metricType == "" {
			metricType = "gauge"
		}
		value, err := valueAsPromFloat(row.Values[metric.ValueColumn])
		if err != nil {
			return fmt.Errorf("metric %q value_column %q: %w", metric.Name, metric.ValueColumn, err)
		}

		if metric.Help != "" {
			if _, ok := seenHelp[metric.Name]; !ok {
				lines = append(lines, fmt.Sprintf("# HELP %s %s", metric.Name, escapeHelp(metric.Help)))
				seenHelp[metric.Name] = struct{}{}
			}
		}
		if _, ok := seenType[metric.Name]; !ok {
			lines = append(lines, fmt.Sprintf("# TYPE %s %s", metric.Name, metricType))
			seenType[metric.Name] = struct{}{}
		}
		lines = append(lines, formatSampleLine(metric.Name, metric.Labels, value))

		if strings.TrimSpace(metric.TimestampMetricName) == "" {
			continue
		}

		rawTimestamp, ok := row.Values[input.TimestampColumn]
		if !ok {
			return fmt.Errorf("metric %q timestamp column %q not found", metric.Name, input.TimestampColumn)
		}
		timestampValue, err := valueAsPromFloat(rawTimestamp)
		if err != nil {
			return fmt.Errorf("metric %q timestamp column %q: %w", metric.Name, input.TimestampColumn, err)
		}

		if metric.TimestampMetricHelp != "" {
			if _, ok := seenHelp[metric.TimestampMetricName]; !ok {
				lines = append(lines, fmt.Sprintf("# HELP %s %s", metric.TimestampMetricName, escapeHelp(metric.TimestampMetricHelp)))
				seenHelp[metric.TimestampMetricName] = struct{}{}
			}
		}
		if _, ok := seenType[metric.TimestampMetricName]; !ok {
			lines = append(lines, fmt.Sprintf("# TYPE %s gauge", metric.TimestampMetricName))
			seenType[metric.TimestampMetricName] = struct{}{}
		}
		lines = append(lines, formatSampleLine(metric.TimestampMetricName, metric.Labels, timestampValue))
	}

	if len(lines) == 0 {
		return nil
	}

	outputPath := filepath.Join(cfg.Directory, cfg.FileName)
	return writeAtomically(outputPath, strings.Join(lines, "\n")+"\n")
}

func writeAtomically(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.WriteString(content); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Chmod(0o644); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}

	return os.Rename(tempPath, path)
}

func valueAsPromFloat(value any) (float64, error) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, fmt.Errorf("invalid float value %v", typed)
		}
		return typed, nil
	case int64:
		return float64(typed), nil
	case int:
		return float64(typed), nil
	case bool:
		if typed {
			return 1, nil
		}
		return 0, nil
	case time.Time:
		return float64(typed.UTC().Unix()), nil
	default:
		return 0, fmt.Errorf("unsupported metric value type %T", value)
	}
}

func formatSampleLine(metricName string, labels map[string]string, value float64) string {
	if len(labels) == 0 {
		return metricName + " " + strconv.FormatFloat(value, 'f', -1, 64)
	}

	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, fmt.Sprintf(`%s="%s"`, key, escapeLabelValue(labels[key])))
	}

	return fmt.Sprintf("%s{%s} %s", metricName, strings.Join(pairs, ","), strconv.FormatFloat(value, 'f', -1, 64))
}

func escapeLabelValue(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)
	return replacer.Replace(value)
}

func escapeHelp(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "\n", `\n`)
	return replacer.Replace(value)
}
