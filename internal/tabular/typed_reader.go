package tabular

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type ColumnType string

const (
	ColumnTypeTimestamp ColumnType = "timestamp"
	ColumnTypeFloat64   ColumnType = "float64"
	ColumnTypeInt64     ColumnType = "int64"
	ColumnTypeString    ColumnType = "string"
	ColumnTypeBool      ColumnType = "bool"
)

type ColumnSpec struct {
	Type ColumnType
}

type TypedReadOptions struct {
	Delimiter        rune
	DecimalComma     bool
	SkipLines        int
	HeaderColumns    []string
	TimestampColumn  string
	TimestampSourceColumns []string
	ColumnSpecs      map[string]ColumnSpec
	TimestampLayouts []string
}

type TypedFileSummary struct {
	Delimiter         rune
	Header            []string
	RowCount          int
	WellFormedRowCount int
	MalformedRowCount int
	MalformedRows     []MalformedRow
	ParseErrorCount   int
	ParseErrors       []ParseError
	PreviewRows       []TypedRow
}

type TypedRow struct {
	LineNumber int
	Raw    []string
	Values map[string]any
}

type ParseError struct {
	LineNumber  int
	ColumnName  string
	ExpectedType ColumnType
	Raw         string
	Error       string
}

func ReadTypedFileSummary(path string, previewRows int, opts TypedReadOptions) (*TypedFileSummary, error) {
	summary, err := ReadFileSummaryWithOptions(path, previewRows, ReadOptions{
		Delimiter:     opts.Delimiter,
		SkipLines:     opts.SkipLines,
		HeaderColumns: opts.HeaderColumns,
	})
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	delimiter := opts.Delimiter
	if delimiter == 0 {
		delimiter = summary.Delimiter
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxScanCapacity)

	typed := &TypedFileSummary{
		Delimiter:          delimiter,
		Header:             append([]string(nil), summary.Header...),
		RowCount:           summary.RowCount,
		WellFormedRowCount: summary.WellFormedRowCount,
		MalformedRowCount:  summary.MalformedRowCount,
		MalformedRows:      append([]MalformedRow(nil), summary.MalformedRows...),
		ParseErrors:        make([]ParseError, 0, min(previewRows, summary.WellFormedRowCount)),
		PreviewRows:        make([]TypedRow, 0, min(previewRows, summary.WellFormedRowCount)),
	}

	lineNumber, err := skipScannerLines(scanner, opts.SkipLines)
	if err != nil {
		return nil, err
	}
	header := append([]string(nil), typed.Header...)
	if len(opts.HeaderColumns) > 0 {
		header = append(header[:0], opts.HeaderColumns...)
	} else {
		headerLine, ok, err := nextNonEmptyLine(scanner)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("empty file")
		}
		lineNumber++
		header = splitLine(headerLine, delimiter)
	}
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		raw := splitLine(line, delimiter)
		if len(raw) != len(header) {
			continue
		}
		values := make(map[string]any, len(header))
		rawByColumn := make(map[string]string, len(header))
		for idx, columnName := range header {
			if idx < len(raw) {
				rawByColumn[columnName] = raw[idx]
			}
		}
		if len(opts.TimestampSourceColumns) > 0 {
			value, joined, err := parseCombinedTimestamp(rawByColumn, opts.TimestampSourceColumns, opts.TimestampLayouts)
			if err != nil {
				typed.ParseErrorCount++
				if len(typed.ParseErrors) < previewRows {
					typed.ParseErrors = append(typed.ParseErrors, ParseError{
						LineNumber:   lineNumber,
						ColumnName:   opts.TimestampColumn,
						ExpectedType: ColumnTypeTimestamp,
						Raw:          joined,
						Error:        err.Error(),
					})
				}
				values[opts.TimestampColumn] = joined
			} else {
				values[opts.TimestampColumn] = value
			}
		}
		for idx, columnName := range header {
			if idx >= len(raw) {
				continue
			}

			if len(opts.TimestampSourceColumns) == 0 && columnName == opts.TimestampColumn {
				value, err := parseValueByType(raw[idx], ColumnTypeTimestamp, opts.DecimalComma, opts.TimestampLayouts)
				if err != nil {
					typed.ParseErrorCount++
					if len(typed.ParseErrors) < previewRows {
						typed.ParseErrors = append(typed.ParseErrors, ParseError{
							LineNumber:   lineNumber,
							ColumnName:   columnName,
							ExpectedType: ColumnTypeTimestamp,
							Raw:          raw[idx],
							Error:        err.Error(),
						})
					}
					values[columnName] = raw[idx]
				} else {
					values[columnName] = value
				}
				continue
			}

			spec, ok := opts.ColumnSpecs[columnName]
			if !ok {
				values[columnName] = inferValue(raw[idx], opts.DecimalComma, opts.TimestampLayouts)
				continue
			}

			value, err := parseValueByType(raw[idx], spec.Type, opts.DecimalComma, opts.TimestampLayouts)
			if err != nil {
				typed.ParseErrorCount++
				if len(typed.ParseErrors) < previewRows {
					typed.ParseErrors = append(typed.ParseErrors, ParseError{
						LineNumber:   lineNumber,
						ColumnName:   columnName,
						ExpectedType: spec.Type,
						Raw:          raw[idx],
						Error:        err.Error(),
					})
				}
				values[columnName] = raw[idx]
				continue
			}
			values[columnName] = value
		}

		if len(typed.PreviewRows) < previewRows {
			typed.PreviewRows = append(typed.PreviewRows, TypedRow{
				LineNumber: lineNumber,
				Raw:    raw,
				Values: values,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return typed, nil
}

func ForEachTypedRow(path string, opts TypedReadOptions, fn func(TypedRow) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxScanCapacity)

	lineNumber, err := skipScannerLines(scanner, opts.SkipLines)
	if err != nil {
		return err
	}

	var header []string
	delimiter := opts.Delimiter
	if len(opts.HeaderColumns) > 0 {
		header = append([]string(nil), opts.HeaderColumns...)
		if delimiter == 0 {
			sampleLine, ok, err := nextNonEmptyLine(scanner)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("empty file")
			}
			lineNumber++
			delimiter = DetectDelimiter(sampleLine)
			raw := splitLine(sampleLine, delimiter)
			if len(raw) == len(header) {
				if err := fn(buildTypedRow(lineNumber, raw, header, opts)); err != nil {
					return err
				}
			}
		}
	} else {
		headerLine, ok, err := nextNonEmptyLine(scanner)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("empty file")
		}
		lineNumber++
		if delimiter == 0 {
			delimiter = DetectDelimiter(headerLine)
		}
		header = splitLine(headerLine, delimiter)
	}

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		raw := splitLine(line, delimiter)
		if len(raw) != len(header) {
			continue
		}

		if err := fn(buildTypedRow(lineNumber, raw, header, opts)); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func buildTypedRow(lineNumber int, raw []string, header []string, opts TypedReadOptions) TypedRow {
	values := make(map[string]any, len(header))
	rawByColumn := make(map[string]string, len(header))
	for idx, columnName := range header {
		if idx < len(raw) {
			rawByColumn[columnName] = raw[idx]
		}
	}
	if len(opts.TimestampSourceColumns) > 0 {
		value, joined, err := parseCombinedTimestamp(rawByColumn, opts.TimestampSourceColumns, opts.TimestampLayouts)
		if err != nil {
			values[opts.TimestampColumn] = joined
		} else {
			values[opts.TimestampColumn] = value
		}
	}
	for idx, columnName := range header {
		if idx >= len(raw) {
			continue
		}

		if len(opts.TimestampSourceColumns) == 0 && columnName == opts.TimestampColumn {
			value, err := parseValueByType(raw[idx], ColumnTypeTimestamp, opts.DecimalComma, opts.TimestampLayouts)
			if err != nil {
				values[columnName] = raw[idx]
			} else {
				values[columnName] = value
			}
			continue
		}

		spec, ok := opts.ColumnSpecs[columnName]
		if !ok {
			values[columnName] = inferValue(raw[idx], opts.DecimalComma, opts.TimestampLayouts)
			continue
		}

		value, err := parseValueByType(raw[idx], spec.Type, opts.DecimalComma, opts.TimestampLayouts)
		if err != nil {
			values[columnName] = raw[idx]
			continue
		}
		values[columnName] = value
	}

	return TypedRow{
		LineNumber: lineNumber,
		Raw:        raw,
		Values:     values,
	}
}

func parseValueByType(raw string, valueType ColumnType, decimalComma bool, layouts []string) (any, error) {
	value := strings.TrimSpace(raw)
	switch valueType {
	case ColumnTypeTimestamp:
		return parseTimestampValue(value, layouts)
	case ColumnTypeFloat64:
		return parseFloatValue(value, decimalComma)
	case ColumnTypeInt64:
		return parseIntValue(value, decimalComma)
	case ColumnTypeString:
		return value, nil
	case ColumnTypeBool:
		return parseBoolValue(value)
	default:
		return nil, fmt.Errorf("unsupported type %q", valueType)
	}
}

func inferValue(raw string, decimalComma bool, layouts []string) any {
	value := strings.TrimSpace(raw)
	if ts, err := parseTimestampValue(value, layouts); err == nil {
		return ts
	}
	if fv, err := parseFloatValue(value, decimalComma); err == nil {
		return fv
	}
	if bv, err := parseBoolValue(value); err == nil {
		return bv
	}
	return value
}

func parseTimestampValue(value string, layouts []string) (time.Time, error) {
	if len(layouts) == 0 {
		layouts = []string{"2006_01_02 15:04:05", time.RFC3339}
	}
	var lastErr error
	for _, layout := range layouts {
		ts, err := time.ParseInLocation(layout, value, time.UTC)
		if err == nil {
			return ts, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

func parseCombinedTimestamp(rawByColumn map[string]string, sourceColumns, layouts []string) (time.Time, string, error) {
	parts := make([]string, 0, len(sourceColumns))
	for _, columnName := range sourceColumns {
		parts = append(parts, strings.TrimSpace(rawByColumn[columnName]))
	}
	joined := strings.Join(parts, " ")
	ts, err := parseTimestampValue(joined, layouts)
	if err != nil {
		return time.Time{}, joined, err
	}
	return ts, joined, nil
}

func parseFloatValue(value string, decimalComma bool) (float64, error) {
	if decimalComma {
		value = strings.ReplaceAll(value, ",", ".")
	}
	return strconv.ParseFloat(value, 64)
}

func parseIntValue(value string, decimalComma bool) (int64, error) {
	if decimalComma {
		value = strings.ReplaceAll(value, ",", ".")
	}
	if strings.Contains(value, ".") {
		fv, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, err
		}
		return int64(fv), nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func parseBoolValue(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "t", "1", "yes", "y", "on":
		return true, nil
	case "false", "f", "0", "no", "n", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool %q", value)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
