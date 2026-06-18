package tabular

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadTypedFileSummaryWebdynsunPreservesBlockContext(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.gz")
	content := strings.Join([]string{
		"SNINV;ADDR23;016",
		"TypeINV;C2-C3-Oeste_INV_GENERIC.ini",
		"2;value_a;value_b",
		"18/06/26-16:30:06;1.5;2.5",
		"SNINV;ADDR24;017",
		"TypeINV;C2-C3-Oeste_INV_GENERIC.ini",
		"2;value_a;value_b",
		"18/06/26-16:31:06;3.5;4.5",
		"",
	}, "\n")

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	gz := gzip.NewWriter(file)
	if _, err := gz.Write([]byte(content)); err != nil {
		t.Fatalf("gzip.Write() error = %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip.Close() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Fatalf("LoadLocation() error = %v", err)
	}

	summary, err := ReadTypedFileSummary(path, 2, TypedReadOptions{
		FileType:               "webdynsun",
		Delimiter:              ';',
		HeaderColumns:          []string{"TIMESTAMP", "value_a", "value_b"},
		TimestampColumn:        "ts",
		TimestampSourceColumns: []string{"TIMESTAMP"},
		TimestampLayouts:       []string{"02/01/06-15:04:05"},
		TimestampLocation:      loc,
		WebdynsunAddressColumn: "addr",
		WebdynsunTypeColumn:    "type_inv",
	})
	if err != nil {
		t.Fatalf("ReadTypedFileSummary() error = %v", err)
	}

	if summary.RowCount != 2 {
		t.Fatalf("RowCount = %d, want 2", summary.RowCount)
	}
	if summary.WellFormedRowCount != 2 {
		t.Fatalf("WellFormedRowCount = %d, want 2", summary.WellFormedRowCount)
	}
	if got := summary.PreviewRows[0].Values["addr"]; got != int64(23) {
		t.Fatalf("addr[0] = %#v, want 23", got)
	}
	if got := summary.PreviewRows[1].Values["addr"]; got != int64(24) {
		t.Fatalf("addr[1] = %#v, want 24", got)
	}
	if got := summary.PreviewRows[0].Values["type_inv"]; got != "C2-C3-Oeste_INV_GENERIC.ini" {
		t.Fatalf("type_inv[0] = %#v, want %q", got, "C2-C3-Oeste_INV_GENERIC.ini")
	}
	if _, ok := summary.PreviewRows[0].Values["ts"].(time.Time); !ok {
		t.Fatalf("ts should be time.Time, got %#v", summary.PreviewRows[0].Values["ts"])
	}
}
