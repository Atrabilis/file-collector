package tabular

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const maxScanCapacity = 1024 * 1024

type FileSummary struct {
	Delimiter         rune
	Header            []string
	RowCount          int
	WellFormedRowCount int
	MalformedRowCount int
	MalformedRows     []MalformedRow
	PreviewRows       [][]string
}

type MalformedRow struct {
	LineNumber      int
	ExpectedColumns int
	ActualColumns   int
	Raw             string
}

type ReadOptions struct {
	Delimiter     rune
	SkipLines     int
	HeaderColumns []string
}

func ReadFileSummary(path string, previewRows int) (*FileSummary, error) {
	return ReadFileSummaryWithOptions(path, previewRows, ReadOptions{})
}

func ReadFileSummaryWithOptions(path string, previewRows int, opts ReadOptions) (*FileSummary, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxScanCapacity)

	delimiter := opts.Delimiter
	lineNumber, err := skipScannerLines(scanner, opts.SkipLines)
	if err != nil {
		return nil, err
	}

	var header []string
	if len(opts.HeaderColumns) > 0 {
		header = append([]string(nil), opts.HeaderColumns...)
		if len(header) < 2 {
			return nil, fmt.Errorf("could not parse header columns")
		}
		if delimiter == 0 {
			sampleLine, ok, err := nextNonEmptyLine(scanner)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("empty file")
			}
			lineNumber++
			delimiter = DetectDelimiter(sampleLine)
			row := splitLine(sampleLine, delimiter)
			if len(row) == len(header) {
				// Count the sampled row as data since there is no header row in the file.
				summary := &FileSummary{
					Delimiter:     delimiter,
					Header:        header,
					MalformedRows: make([]MalformedRow, 0, max(0, previewRows)),
					PreviewRows:   make([][]string, 0, max(0, previewRows)),
					RowCount:      1,
				}
				summary.WellFormedRowCount = 1
				if len(summary.PreviewRows) < previewRows {
					summary.PreviewRows = append(summary.PreviewRows, row)
				}
				if err := consumeRows(scanner, delimiter, header, previewRows, lineNumber, summary); err != nil {
					return nil, err
				}
				return summary, nil
			}

			summary := &FileSummary{
				Delimiter:     delimiter,
				Header:        header,
				MalformedRows: make([]MalformedRow, 0, max(0, previewRows)),
				PreviewRows:   make([][]string, 0, max(0, previewRows)),
				RowCount:      1,
			}
			summary.MalformedRowCount = 1
			if len(summary.MalformedRows) < previewRows {
				summary.MalformedRows = append(summary.MalformedRows, MalformedRow{
					LineNumber:      lineNumber,
					ExpectedColumns: len(header),
					ActualColumns:   len(row),
					Raw:             sampleLine,
				})
			}
			if err := consumeRows(scanner, delimiter, header, previewRows, lineNumber, summary); err != nil {
				return nil, err
			}
			return summary, nil
		}
	} else {
		headerLine, ok, err := nextNonEmptyLine(scanner)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("empty file")
		}
		lineNumber++
		if delimiter == 0 {
			delimiter = DetectDelimiter(headerLine)
		}
		header = splitLine(headerLine, delimiter)
		if len(header) < 2 {
			return nil, fmt.Errorf("could not parse header columns")
		}
	}

	summary := &FileSummary{
		Delimiter:     delimiter,
		Header:        header,
		MalformedRows: make([]MalformedRow, 0, max(0, previewRows)),
		PreviewRows:   make([][]string, 0, max(0, previewRows)),
	}

	if err := consumeRows(scanner, delimiter, header, previewRows, lineNumber, summary); err != nil {
		return nil, err
	}

	return summary, nil
}

func consumeRows(scanner *bufio.Scanner, delimiter rune, header []string, previewRows int, lineNumber int, summary *FileSummary) error {
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		row := splitLine(line, delimiter)
		summary.RowCount++
		if len(row) != len(header) {
			summary.MalformedRowCount++
			if len(summary.MalformedRows) < previewRows {
				summary.MalformedRows = append(summary.MalformedRows, MalformedRow{
					LineNumber:      lineNumber,
					ExpectedColumns: len(header),
					ActualColumns:   len(row),
					Raw:             line,
				})
			}
			continue
		}
		summary.WellFormedRowCount++

		if len(summary.PreviewRows) < previewRows {
			summary.PreviewRows = append(summary.PreviewRows, row)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func DetectDelimiter(header string) rune {
	candidates := []rune{';', ',', '\t', '|'}
	best := ';'
	bestCount := -1

	for _, candidate := range candidates {
		count := strings.Count(header, string(candidate))
		if count > bestCount {
			best = candidate
			bestCount = count
		}
	}

	return best
}

func splitLine(line string, delimiter rune) []string {
	parts := strings.Split(line, string(delimiter))
	for idx, part := range parts {
		parts[idx] = strings.Trim(strings.TrimSpace(part), `"`)
	}
	return parts
}

func nextNonEmptyLine(scanner *bufio.Scanner) (string, bool, error) {
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		return line, true, nil
	}

	if err := scanner.Err(); err != nil {
		return "", false, err
	}

	return "", false, nil
}

func skipScannerLines(scanner *bufio.Scanner, skipLines int) (int, error) {
	lineNumber := 0
	for skipped := 0; skipped < skipLines; {
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return lineNumber, err
			}
			return lineNumber, nil
		}
		lineNumber++
		skipped++
	}
	return lineNumber, nil
}
