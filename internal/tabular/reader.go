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
	Delimiter rune
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

	headerLine, ok, err := nextNonEmptyLine(scanner)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("empty file")
	}

	delimiter := opts.Delimiter
	if delimiter == 0 {
		delimiter = DetectDelimiter(headerLine)
	}
	header := splitLine(headerLine, delimiter)
	if len(header) < 2 {
		return nil, fmt.Errorf("could not parse header columns")
	}

	summary := &FileSummary{
		Delimiter:     delimiter,
		Header:        header,
		MalformedRows: make([]MalformedRow, 0, max(0, previewRows)),
		PreviewRows:   make([][]string, 0, max(0, previewRows)),
	}

	lineNumber := 1
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
		return nil, err
	}

	return summary, nil
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
		parts[idx] = strings.TrimSpace(part)
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
