package promtextfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atamostec/file-collector/internal/config"
	"github.com/atamostec/file-collector/internal/tabular"
)

func TestExportInputMetricsWritesAtomicPrometheusTextfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	input := config.Input{
		Name:            "psda__meteo_6852",
		TimestampColumn: "ts",
		PrometheusTextfile: config.PrometheusTextfileConfig{
			Enabled:   true,
			Directory: dir,
			FileName:  "psda__meteo_6852.prom",
			Metrics: []config.PrometheusTextfileMetricConfig{
				{
					Name:                "atamostec_radiometry_dni_avg_watts_per_square_meter",
					Help:                "Latest DNI average from file-collector.",
					ValueColumn:         "DNI_Avg",
					Labels:              map[string]string{"plant": "psda", "sensor": "meteo6852"},
					TimestampMetricName: "atamostec_radiometry_sample_timestamp_seconds",
					TimestampMetricHelp: "Timestamp of the latest source sample.",
				},
			},
		},
	}

	row := tabular.TypedRow{
		Values: map[string]any{
			"DNI_Avg": 523.4,
			"ts":      time.Unix(1719055500, 0).UTC(),
		},
	}

	if err := ExportInputMetrics(input, row); err != nil {
		t.Fatalf("ExportInputMetrics() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "psda__meteo_6852.prom"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	got := string(content)
	wantFragments := []string{
		`# HELP atamostec_radiometry_dni_avg_watts_per_square_meter Latest DNI average from file-collector.`,
		`# TYPE atamostec_radiometry_dni_avg_watts_per_square_meter gauge`,
		`atamostec_radiometry_dni_avg_watts_per_square_meter{plant="psda",sensor="meteo6852"} 523.4`,
		`# HELP atamostec_radiometry_sample_timestamp_seconds Timestamp of the latest source sample.`,
		`# TYPE atamostec_radiometry_sample_timestamp_seconds gauge`,
		`atamostec_radiometry_sample_timestamp_seconds{plant="psda",sensor="meteo6852"} 1719055500`,
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(got, fragment) {
			t.Fatalf("content missing %q\nfull content:\n%s", fragment, got)
		}
	}
}

func TestExportInputMetricsReplacesPreviousContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "psda__meteo_6852.prom")
	if err := os.WriteFile(path, []byte("obsolete_metric 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	input := config.Input{
		Name:            "psda__meteo_6852",
		TimestampColumn: "ts",
		PrometheusTextfile: config.PrometheusTextfileConfig{
			Enabled:   true,
			Directory: dir,
			FileName:  "psda__meteo_6852.prom",
			Metrics: []config.PrometheusTextfileMetricConfig{
				{
					Name:        "atamostec_radiometry_dni_avg_watts_per_square_meter",
					ValueColumn: "DNI_Avg",
				},
			},
		},
	}

	row := tabular.TypedRow{
		Values: map[string]any{
			"DNI_Avg": 401.5,
			"ts":      time.Unix(1719055500, 0).UTC(),
		},
	}

	if err := ExportInputMetrics(input, row); err != nil {
		t.Fatalf("ExportInputMetrics() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(content)
	if strings.Contains(got, "obsolete_metric") {
		t.Fatalf("content still contains obsolete metric:\n%s", got)
	}
	if !strings.Contains(got, "atamostec_radiometry_dni_avg_watts_per_square_meter 401.5") {
		t.Fatalf("content missing rewritten metric:\n%s", got)
	}
}
