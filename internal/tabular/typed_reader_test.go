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

func TestReadTypedFileSummaryBuildsCombinedTimestamp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.iud")
	content := "Date\ttime\tPmax\n17.12.2019\t14:22:03\t272.21\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	summary, err := ReadTypedFileSummary(path, 1, TypedReadOptions{
		Delimiter:              '\t',
		TimestampColumn:        "ts",
		TimestampSourceColumns: []string{"Date", "time"},
		TimestampLayouts:       []string{"02.01.2006 15:04:05"},
		ColumnSpecs: map[string]ColumnSpec{
			"Pmax": {Type: ColumnTypeFloat64},
		},
	})
	if err != nil {
		t.Fatalf("ReadTypedFileSummary() error = %v", err)
	}

	got, ok := summary.PreviewRows[0].Values["ts"].(time.Time)
	if !ok {
		t.Fatalf("ts should be time.Time, got %#v", summary.PreviewRows[0].Values["ts"])
	}
	want := time.Date(2019, time.December, 17, 14, 22, 3, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("ts = %s, want %s", got, want)
	}
}
