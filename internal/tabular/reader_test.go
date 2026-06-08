package tabular

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectDelimiter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
		want   rune
	}{
		{name: "semicolon", header: "TimeStamp;A;B;C", want: ';'},
		{name: "comma", header: "timestamp,a,b,c", want: ','},
		{name: "tab", header: "timestamp\tvalue1\tvalue2", want: '\t'},
		{name: "pipe", header: "timestamp|value1|value2", want: '|'},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := DetectDelimiter(test.header); got != test.want {
				t.Fatalf("DetectDelimiter() = %q, want %q", string(got), string(test.want))
			}
		})
	}
}

func TestReadFileSummary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	content := "TimeStamp;A;B\n2024_06_04 00:00:03;1,1;2,2\n2024_06_04 00:00:08;3,3;4,4\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	summary, err := ReadFileSummary(path, 2)
	if err != nil {
		t.Fatalf("ReadFileSummary() error = %v", err)
	}

	if summary.Delimiter != ';' {
		t.Fatalf("Delimiter = %q, want %q", string(summary.Delimiter), ";")
	}
	if got := len(summary.Header); got != 3 {
		t.Fatalf("len(Header) = %d, want 3", got)
	}
	if summary.RowCount != 2 {
		t.Fatalf("RowCount = %d, want 2", summary.RowCount)
	}
	if summary.WellFormedRowCount != 2 {
		t.Fatalf("WellFormedRowCount = %d, want 2", summary.WellFormedRowCount)
	}
	if summary.MalformedRowCount != 0 {
		t.Fatalf("MalformedRowCount = %d, want 0", summary.MalformedRowCount)
	}
	if got := len(summary.PreviewRows); got != 2 {
		t.Fatalf("len(PreviewRows) = %d, want 2", got)
	}
}

func TestReadFileSummaryWithExplicitDelimiter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	content := "TimeStamp|A|B\n2024_06_04 00:00:03|1,1|2,2\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	summary, err := ReadFileSummaryWithOptions(path, 1, ReadOptions{Delimiter: '|'})
	if err != nil {
		t.Fatalf("ReadFileSummaryWithOptions() error = %v", err)
	}

	if summary.Delimiter != '|' {
		t.Fatalf("Delimiter = %q, want %q", string(summary.Delimiter), "|")
	}
	if got := len(summary.Header); got != 3 {
		t.Fatalf("len(Header) = %d, want 3", got)
	}
}

func TestReadFileSummaryTracksMalformedRows(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	content := "TimeStamp;A;B\n2024_06_04 00:00:03;1,1;2,2\n2024_06_04 00:00:08;3,3\n2024_06_04 00:00:13;5,5;6,6\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	summary, err := ReadFileSummary(path, 2)
	if err != nil {
		t.Fatalf("ReadFileSummary() error = %v", err)
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
	if len(summary.MalformedRows) != 1 {
		t.Fatalf("len(MalformedRows) = %d, want 1", len(summary.MalformedRows))
	}
	if summary.MalformedRows[0].LineNumber != 3 {
		t.Fatalf("MalformedRows[0].LineNumber = %d, want 3", summary.MalformedRows[0].LineNumber)
	}
	if summary.MalformedRows[0].ActualColumns != 2 {
		t.Fatalf("MalformedRows[0].ActualColumns = %d, want 2", summary.MalformedRows[0].ActualColumns)
	}
	if got := len(summary.PreviewRows); got != 2 {
		t.Fatalf("len(PreviewRows) = %d, want 2", got)
	}
}
