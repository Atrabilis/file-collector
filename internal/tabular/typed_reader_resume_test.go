package tabular

import (
	"os"
	"path/filepath"
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
