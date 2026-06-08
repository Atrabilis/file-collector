package tabular

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadTypedFileSummaryWithColumnOverrides(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	content := "TimeStamp;Value;Status\n2024_06_04 00:00:03;49,91;ON\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	summary, err := ReadTypedFileSummary(path, 1, TypedReadOptions{
		Delimiter:       ';',
		DecimalComma:    true,
		TimestampColumn: "TimeStamp",
		ColumnSpecs: map[string]ColumnSpec{
			"Value":  {Type: ColumnTypeFloat64},
			"Status": {Type: ColumnTypeString},
		},
	})
	if err != nil {
		t.Fatalf("ReadTypedFileSummary() error = %v", err)
	}

	if got := summary.PreviewRows[0].Values["Value"]; got != 49.91 {
		t.Fatalf("Value = %#v, want 49.91", got)
	}
	if got := summary.PreviewRows[0].Values["Status"]; got != "ON" {
		t.Fatalf("Status = %#v, want %q", got, "ON")
	}
	if _, ok := summary.PreviewRows[0].Values["TimeStamp"].(time.Time); !ok {
		t.Fatalf("TimeStamp should be time.Time, got %#v", summary.PreviewRows[0].Values["TimeStamp"])
	}
}

func TestReadTypedFileSummarySkipsMalformedRowsAndTracksThem(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	content := "TimeStamp;Value;Status\n2024_06_04 00:00:03;49,91;ON\n2024_06_04 00:00:08;50,01\n2024_06_04 00:00:13;50,12;OFF\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	summary, err := ReadTypedFileSummary(path, 5, TypedReadOptions{
		Delimiter:       ';',
		DecimalComma:    true,
		TimestampColumn: "TimeStamp",
		ColumnSpecs: map[string]ColumnSpec{
			"Value":  {Type: ColumnTypeFloat64},
			"Status": {Type: ColumnTypeString},
		},
	})
	if err != nil {
		t.Fatalf("ReadTypedFileSummary() error = %v", err)
	}

	if summary.RowCount != 3 {
		t.Fatalf("RowCount = %d, want 3", summary.RowCount)
	}
	if summary.WellFormedRowCount != 2 {
		t.Fatalf("WellFormedRowCount = %d, want 2", summary.WellFormedRowCount)
	}
	if summary.MalformedRowCount != 1 {
		t.Fatalf("MalformedRowCount = %d, want 1", summary.MalformedRowCount)
	}
	if len(summary.PreviewRows) != 2 {
		t.Fatalf("len(PreviewRows) = %d, want 2", len(summary.PreviewRows))
	}
	if summary.MalformedRows[0].LineNumber != 3 {
		t.Fatalf("MalformedRows[0].LineNumber = %d, want 3", summary.MalformedRows[0].LineNumber)
	}
}

func TestReadTypedFileSummaryTracksParseErrorsForExplicitTypes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	content := "TimeStamp;Value;Status\nnot-a-timestamp;abc;ON\n2024_06_04 00:00:13;50,12;OFF\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	summary, err := ReadTypedFileSummary(path, 5, TypedReadOptions{
		Delimiter:       ';',
		DecimalComma:    true,
		TimestampColumn: "TimeStamp",
		ColumnSpecs: map[string]ColumnSpec{
			"Value":  {Type: ColumnTypeFloat64},
			"Status": {Type: ColumnTypeString},
		},
	})
	if err != nil {
		t.Fatalf("ReadTypedFileSummary() error = %v", err)
	}

	if summary.ParseErrorCount != 2 {
		t.Fatalf("ParseErrorCount = %d, want 2", summary.ParseErrorCount)
	}
	if len(summary.ParseErrors) != 2 {
		t.Fatalf("len(ParseErrors) = %d, want 2", len(summary.ParseErrors))
	}
	if summary.ParseErrors[0].ColumnName != "TimeStamp" {
		t.Fatalf("ParseErrors[0].ColumnName = %q, want %q", summary.ParseErrors[0].ColumnName, "TimeStamp")
	}
	if summary.ParseErrors[1].ColumnName != "Value" {
		t.Fatalf("ParseErrors[1].ColumnName = %q, want %q", summary.ParseErrors[1].ColumnName, "Value")
	}
}
