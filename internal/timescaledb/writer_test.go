package timescaledb

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atamostec/file-collector/internal/config"
	"github.com/atamostec/file-collector/internal/tabular"
)

func TestBuildInsertPayloadNormalizesColumns(t *testing.T) {
	t.Parallel()

	columns, values, err := buildInsertPayload(
		config.Input{TimestampColumn: "TimeStamp"},
		"/tmp/20260604_DCpower.txt",
		tabular.TypedRow{
			LineNumber: 2,
			Values: map[string]any{
				"TimeStamp": time.Date(2026, time.June, 4, 0, 0, 2, 0, time.UTC),
				"2VE201(V)": 123.4,
				"2PE201(W)": 456.7,
				"THD_V1(%)": 7.8,
			},
		},
		time.Date(2026, time.June, 4, 0, 0, 2, 0, time.UTC),
		map[string]struct{}{
			"ts":                 {},
			"source_file":        {},
			"source_line_number": {},
			"flags":              {},
			"2_pe_201_w":         {},
			"2_ve_201_v":         {},
			"thd_v_1_pct":        {},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("buildInsertPayload() error = %v", err)
	}

	gotCols := strings.Join(columns, ",")
	wantCols := "ts,source_file,source_line_number,flags,2_pe_201_w,2_ve_201_v,thd_v_1_pct"
	if gotCols != wantCols {
		t.Fatalf("columns = %q, want %q", gotCols, wantCols)
	}
	if values[1] != filepath.Base("/tmp/20260604_DCpower.txt") {
		t.Fatalf("source_file = %#v", values[1])
	}
}

func TestBuildInsertPayloadSkipsTimestampSourceColumns(t *testing.T) {
	t.Parallel()

	columns, _, err := buildInsertPayload(
		config.Input{
			TimestampColumn:        "ts",
			TimestampSourceColumns: []string{"Date", "time"},
		},
		"/tmp/20191217HET1_fixed_1MD410.iud",
		tabular.TypedRow{
			LineNumber: 2,
			Values: map[string]any{
				"ts":   time.Date(2019, time.December, 17, 14, 22, 3, 0, time.UTC),
				"Date": "17.12.2019",
				"time": "14:22:03",
				"Pmax": 272.21,
			},
		},
		time.Date(2019, time.December, 17, 14, 22, 3, 0, time.UTC),
		map[string]struct{}{
			"ts":                 {},
			"source_file":        {},
			"source_line_number": {},
			"flags":              {},
			"pmax":               {},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("buildInsertPayload() error = %v", err)
	}

	gotCols := strings.Join(columns, ",")
	wantCols := "ts,source_file,source_line_number,flags,pmax"
	if gotCols != wantCols {
		t.Fatalf("columns = %q, want %q", gotCols, wantCols)
	}
}

func TestBuildInsertPayloadSkipsColumnsMissingFromDestination(t *testing.T) {
	t.Parallel()

	columns, _, err := buildInsertPayload(
		config.Input{TimestampColumn: "ts"},
		"/tmp/20250930HET1_fixed_1MD410.iud",
		tabular.TypedRow{
			LineNumber: 4,
			Values: map[string]any{
				"ts":       time.Date(2025, time.September, 30, 13, 12, 25, 0, time.UTC),
				"015-2017": 675.44,
				"063-2017": 688.14,
			},
		},
		time.Date(2025, time.September, 30, 13, 12, 25, 0, time.UTC),
		map[string]struct{}{
			"ts":                 {},
			"source_file":        {},
			"source_line_number": {},
			"flags":              {},
			"015_2017":           {},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("buildInsertPayload() error = %v", err)
	}

	gotCols := strings.Join(columns, ",")
	wantCols := `ts,source_file,source_line_number,flags,015_2017`
	if gotCols != wantCols {
		t.Fatalf("columns = %q, want %q", gotCols, wantCols)
	}
}

func TestBuildInsertPayloadConvertsNaNToNil(t *testing.T) {
	t.Parallel()

	stats := &fileWriteStats{
		sourceColumns:             make(map[string]struct{}),
		skippedDestinationColumns: make(map[string]struct{}),
	}

	columns, values, err := buildInsertPayload(
		config.Input{TimestampColumn: "ts"},
		"/tmp/20250930HET1_fixed_1MD410.iud",
		tabular.TypedRow{
			LineNumber: 4,
			Values: map[string]any{
				"ts":     time.Date(2025, time.September, 30, 13, 12, 25, 0, time.UTC),
				"FF_raw": math.NaN(),
			},
		},
		time.Date(2025, time.September, 30, 13, 12, 25, 0, time.UTC),
		map[string]struct{}{
			"ts":                 {},
			"source_file":        {},
			"source_line_number": {},
			"flags":              {},
			"ff_raw":             {},
		},
		stats,
	)
	if err != nil {
		t.Fatalf("buildInsertPayload() error = %v", err)
	}

	gotCols := strings.Join(columns, ",")
	wantCols := "ts,source_file,source_line_number,flags,ff_raw"
	if gotCols != wantCols {
		t.Fatalf("columns = %q, want %q", gotCols, wantCols)
	}
	if values[len(values)-1] != nil {
		t.Fatalf("last value = %#v, want nil", values[len(values)-1])
	}
	if stats.nanToNullCount != 1 {
		t.Fatalf("nanToNullCount = %d, want 1", stats.nanToNullCount)
	}
}

func TestBuildInsertPayloadTracksSkippedDestinationColumns(t *testing.T) {
	t.Parallel()

	stats := &fileWriteStats{
		sourceColumns:             make(map[string]struct{}),
		skippedDestinationColumns: make(map[string]struct{}),
	}

	_, _, err := buildInsertPayload(
		config.Input{TimestampColumn: "ts"},
		"/tmp/20250930HET1_fixed_1MD410.iud",
		tabular.TypedRow{
			LineNumber: 4,
			Values: map[string]any{
				"ts":       time.Date(2025, time.September, 30, 13, 12, 25, 0, time.UTC),
				"015-2017": 675.44,
				"063-2017": 688.14,
			},
		},
		time.Date(2025, time.September, 30, 13, 12, 25, 0, time.UTC),
		map[string]struct{}{
			"ts":                 {},
			"source_file":        {},
			"source_line_number": {},
			"flags":              {},
			"015_2017":           {},
		},
		stats,
	)
	if err != nil {
		t.Fatalf("buildInsertPayload() error = %v", err)
	}

	if _, ok := stats.sourceColumns["015_2017"]; !ok {
		t.Fatalf("sourceColumns missing 015_2017")
	}
	if _, ok := stats.sourceColumns["063_2017"]; !ok {
		t.Fatalf("sourceColumns missing 063_2017")
	}
	if _, ok := stats.skippedDestinationColumns["063_2017"]; !ok {
		t.Fatalf("skippedDestinationColumns missing 063_2017")
	}
}

func TestIsCollectorManagedColumn(t *testing.T) {
	t.Parallel()

	for _, column := range []string{"ts", "source_file", "source_line_number", "flags", "ingested_at"} {
		if !isCollectorManagedColumn(column) {
			t.Fatalf("isCollectorManagedColumn(%q) = false, want true", column)
		}
	}
	if isCollectorManagedColumn("pmax") {
		t.Fatalf("isCollectorManagedColumn(pmax) = true, want false")
	}
}

func TestBuildInsertStatementQuotesNumericIdentifiers(t *testing.T) {
	t.Parallel()

	stmt := buildInsertStatement("ua", "cdaq2_dc_power", []string{
		"ts",
		"source_file",
		"2_ve_201_v",
	}, 2, "do_update", []string{"ts"})

	if !strings.Contains(stmt, `INSERT INTO ua.cdaq2_dc_power (ts, source_file, "2_ve_201_v") VALUES ($1, $2, $3), ($4, $5, $6)`) {
		t.Fatalf("statement insert columns not quoted as expected: %s", stmt)
	}
	if !strings.Contains(stmt, `ON CONFLICT (ts) DO UPDATE SET source_file = EXCLUDED.source_file, "2_ve_201_v" = EXCLUDED."2_ve_201_v"`) {
		t.Fatalf("statement conflict clause not quoted as expected: %s", stmt)
	}
}

func TestBuildInsertStatementDoNothing(t *testing.T) {
	t.Parallel()

	stmt := buildInsertStatement("ua", "cdaq2_dc_power", []string{
		"ts",
		"source_file",
		"2_ve_201_v",
	}, 1, "do_nothing", []string{"ts"})

	if !strings.Contains(stmt, `ON CONFLICT (ts) DO NOTHING`) {
		t.Fatalf("statement conflict clause not set to DO NOTHING: %s", stmt)
	}
}

func TestBuildInsertStatementUsesCompositePrimaryKey(t *testing.T) {
	t.Parallel()

	stmt := buildInsertStatement("psda", "meteo_22568", []string{
		"ts",
		"source_file",
		"source_line_number",
		"record",
	}, 1, "do_update", []string{"ts", "source_file", "source_line_number"})

	if !strings.Contains(stmt, `ON CONFLICT (ts, source_file, source_line_number) DO UPDATE SET record = EXCLUDED.record`) {
		t.Fatalf("statement conflict clause did not use composite primary key as expected: %s", stmt)
	}
	if strings.Contains(stmt, `source_file = EXCLUDED.source_file`) || strings.Contains(stmt, `source_line_number = EXCLUDED.source_line_number`) {
		t.Fatalf("statement should not update primary key columns: %s", stmt)
	}
}

func TestSameColumns(t *testing.T) {
	t.Parallel()

	if !sameColumns([]string{"ts", "value"}, []string{"ts", "value"}) {
		t.Fatalf("sameColumns() = false, want true")
	}
	if sameColumns([]string{"ts", "value"}, []string{"ts", "other"}) {
		t.Fatalf("sameColumns() = true, want false")
	}
}

func TestEffectiveBatchSizeUsesConfiguredValueWhenSafe(t *testing.T) {
	t.Parallel()

	got := effectiveBatchSize(5000, 10)
	if got != 5000 {
		t.Fatalf("effectiveBatchSize() = %d, want 5000", got)
	}
}

func TestEffectiveBatchSizeCapsByParameterLimit(t *testing.T) {
	t.Parallel()

	got := effectiveBatchSize(5000, 16)
	want := 4095
	if got != want {
		t.Fatalf("effectiveBatchSize() = %d, want %d", got, want)
	}
}

func TestEffectiveBatchSizeReturnsOneWhenColumnCountExceedsParameterLimit(t *testing.T) {
	t.Parallel()

	got := effectiveBatchSize(5000, maxPostgresParameters+10)
	if got != 1 {
		t.Fatalf("effectiveBatchSize() = %d, want 1", got)
	}
}
