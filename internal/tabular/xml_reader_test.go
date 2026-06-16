package tabular

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadXMLFileSummaryParsesAttributes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.xml")
	content := `<?xml version="1.0" encoding="UTF-8"?>
<IV_Curves>
  <Version>1.00</Version>
  <Curve>
    <Name>F1</Name>
    <Temp1 tc_type="T">
      <Value units="C">61.853882</Value>
      <InputSource>THERMOCOUPLE</InputSource>
    </Temp1>
  </Curve>
</IV_Curves>`

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	summary, err := ReadTypedFileSummary(path, 1, TypedReadOptions{
		FileType:          "xml",
		TimestampColumn:   "ts",
		TimestampLayouts:  []string{"2006-01-02T15:04:05"},
		XMLFields: []XMLFieldSpec{
			{Name: "name", Path: "Curve.Name", Type: ColumnTypeString},
			{Name: "temp_1_value", Path: "Curve.Temp1.Value", Type: ColumnTypeFloat64},
			{Name: "temp_1_value_units", Path: "Curve.Temp1.Value@units", Type: ColumnTypeString},
			{Name: "temp_1_tc_type", Path: "Curve.Temp1@tc_type", Type: ColumnTypeString},
		},
	})
	if err != nil {
		t.Fatalf("ReadTypedFileSummary() error = %v", err)
	}

	if got := len(summary.Header); got != 4 {
		t.Fatalf("len(Header) = %d, want 4", got)
	}
	if got := len(summary.PreviewRows); got != 1 {
		t.Fatalf("len(PreviewRows) = %d, want 1", got)
	}
	row := summary.PreviewRows[0]
	if got, want := row.Values["temp_1_value"], 61.853882; got != want {
		t.Fatalf("temp_1_value = %v, want %v", got, want)
	}
	if got, want := row.Values["temp_1_value_units"], "C"; got != want {
		t.Fatalf("temp_1_value_units = %v, want %v", got, want)
	}
	if got, want := row.Values["temp_1_tc_type"], "T"; got != want {
		t.Fatalf("temp_1_tc_type = %v, want %v", got, want)
	}
}
