package timescaledb

import (
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
				"TimeStamp":  time.Date(2026, time.June, 4, 0, 0, 2, 0, time.UTC),
				"2VE201(V)":  123.4,
				"2PE201(W)":  456.7,
				"THD_V1(%)":  7.8,
			},
		},
		time.Date(2026, time.June, 4, 0, 0, 2, 0, time.UTC),
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

func TestBuildInsertStatementQuotesNumericIdentifiers(t *testing.T) {
	t.Parallel()

	stmt := buildInsertStatement("ua", "cdaq2_dc_power", []string{
		"ts",
		"source_file",
		"2_ve_201_v",
	}, "do_update")

	if !strings.Contains(stmt, `INSERT INTO ua.cdaq2_dc_power (ts, source_file, "2_ve_201_v")`) {
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
	}, "do_nothing")

	if !strings.Contains(stmt, `ON CONFLICT (ts) DO NOTHING`) {
		t.Fatalf("statement conflict clause not set to DO NOTHING: %s", stmt)
	}
}
