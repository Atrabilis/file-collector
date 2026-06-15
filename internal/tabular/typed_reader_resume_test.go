package tabular

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestForEachTypedRowResumesFromOffset(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "CRx_22568_Meteo.dat")
	content := "TIMESTAMP;BP_kPa;SlrW\n2026-06-12 10:00:00;891.2;101.5\n2026-06-12 10:01:00;891.3;102.0\n2026-06-12 10:02:00;891.4;103.1\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	opts := TypedReadOptions{
		TimestampColumn:  "TIMESTAMP",
		TimestampLayouts: []string{"2006-01-02 15:04:05"},
	}

	var rows []TypedRow
	if err := ForEachTypedRow(path, opts, func(row TypedRow) error {
		rows = append(rows, row)
		return nil
	}); err != nil {
		t.Fatalf("ForEachTypedRow() error = %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}

	resumeOpts := opts
	resumeOpts.StartOffset = rows[1].NextByteOffset
	resumeOpts.StartLineNumber = rows[1].LineNumber

	var resumed []TypedRow
	if err := ForEachTypedRow(path, resumeOpts, func(row TypedRow) error {
		resumed = append(resumed, row)
		return nil
	}); err != nil {
		t.Fatalf("ForEachTypedRow(resume) error = %v", err)
	}
	if len(resumed) != 1 {
		t.Fatalf("len(resumed) = %d, want 1", len(resumed))
	}
	if resumed[0].LineNumber != 4 {
		t.Fatalf("resumed line number = %d, want 4", resumed[0].LineNumber)
	}
	if got := resumed[0].Values["SlrW"]; got != float64(103.1) {
		t.Fatalf("resumed value = %#v, want 103.1", got)
	}
	if ts, ok := resumed[0].Values["TIMESTAMP"].(time.Time); !ok || ts.Format("2006-01-02 15:04:05") != "2026-06-12 10:02:00" {
		t.Fatalf("resumed timestamp = %#v", resumed[0].Values["TIMESTAMP"])
	}
}

func TestForEachTypedRowSkipsCorruptRowsAndReportsThem(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "CRx_22568_Meteo.dat")
	content := strings.Join([]string{
		"TIMESTAMP;BP_kPa;SlrW",
		"2026-06-12 10:00:00;891.2;101.5",
		"\x00\x00\x00\"2026-06-12 10:01:00\";891.3;102.0",
		"bad-ts;891.4;103.1",
		"2026-06-12 10:02:00;891.5",
		"2026-06-12 10:03:00;891.6;104.2",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	opts := TypedReadOptions{
		TimestampColumn:  "TIMESTAMP",
		TimestampLayouts: []string{"2006-01-02 15:04:05"},
	}

	var skipped []SkippedRow
	opts.OnSkippedRow = func(row SkippedRow) {
		skipped = append(skipped, row)
	}

	var rows []TypedRow
	if err := ForEachTypedRow(path, opts, func(row TypedRow) error {
		rows = append(rows, row)
		return nil
	}); err != nil {
		t.Fatalf("ForEachTypedRow() error = %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].LineNumber != 2 || rows[1].LineNumber != 6 {
		t.Fatalf("row line numbers = %#v, want [2 6]", []int{rows[0].LineNumber, rows[1].LineNumber})
	}
	if len(skipped) != 3 {
		t.Fatalf("len(skipped) = %d, want 3", len(skipped))
	}
	if skipped[0].LineNumber != 3 || skipped[0].Reason != "line contains NUL bytes" {
		t.Fatalf("skipped[0] = %#v", skipped[0])
	}
	if skipped[1].LineNumber != 4 || skipped[1].Reason != `timestamp column "TIMESTAMP" is not parsed as time` {
		t.Fatalf("skipped[1] = %#v", skipped[1])
	}
	if skipped[2].LineNumber != 5 || skipped[2].Reason != "column count mismatch expected=3 actual=2" {
		t.Fatalf("skipped[2] = %#v", skipped[2])
	}
}
