package tabular

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type webdynsunState struct {
	addr     int64
	hasAddr  bool
	typeInv  string
	hasType  bool
}

func readWebdynsunFileSummary(path string, previewRows int, opts TypedReadOptions) (*TypedFileSummary, error) {
	summary := &TypedFileSummary{
		Delimiter:          opts.Delimiter,
		Header:             append([]string(nil), opts.HeaderColumns...),
		MalformedRows:      make([]MalformedRow, 0, max(0, previewRows)),
		ParseErrors:        make([]ParseError, 0, max(0, previewRows)),
		PreviewRows:        make([]TypedRow, 0, max(0, previewRows)),
	}
	err := forEachWebdynsunRowInternal(path, opts, func(row TypedRow) error {
		summary.RowCount++
		summary.WellFormedRowCount++
		if len(summary.PreviewRows) < previewRows {
			summary.PreviewRows = append(summary.PreviewRows, row)
		}
		return nil
	}, summary)
	if err != nil {
		return nil, err
	}
	return summary, nil
}

func forEachWebdynsunRow(path string, opts TypedReadOptions, fn func(TypedRow) error) error {
	return forEachWebdynsunRowInternal(path, opts, fn, nil)
}

func forEachWebdynsunRowInternal(path string, opts TypedReadOptions, fn func(TypedRow) error, summary *TypedFileSummary) error {
	if len(opts.HeaderColumns) == 0 {
		return fmt.Errorf("webdynsun file_type requires header_columns")
	}

	file, err := openWebdynsunFile(path)
	if err != nil {
		return err
	}
	defer file.Close()

	header := append([]string(nil), opts.HeaderColumns...)
	delimiter := opts.Delimiter
	if delimiter == 0 {
		delimiter = ';'
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxScanCapacity)

	lineNumber := 0
	currentOffset := int64(0)
	state := webdynsunState{}

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		currentOffset += int64(len(scanner.Bytes()) + 1)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "SNINV;ADDR") {
			state.updateAddr(line)
			continue
		}
		if strings.HasPrefix(line, "TypeINV;") {
			state.updateType(line)
			continue
		}

		raw := splitLine(line, delimiter)
		if len(raw) != len(header) {
			continue
		}
		if isWebdynsunBlockHeader(raw) {
			continue
		}

		row := buildTypedRow(lineNumber, raw, header, opts)
		row.ByteOffset = currentOffset - int64(len(scanner.Bytes())+1)
		row.NextByteOffset = currentOffset
		if !rowHasParsedTimestamp(row, opts.TimestampColumn) {
			continue
		}
		state.inject(row.Values, opts)
		if err := fn(row); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (s *webdynsunState) updateAddr(line string) {
	parts := strings.Split(line, ";")
	if len(parts) < 2 {
		return
	}
	addr := strings.TrimPrefix(strings.TrimSpace(parts[1]), "ADDR")
	value, err := strconv.ParseInt(addr, 10, 64)
	if err != nil {
		return
	}
	s.addr = value
	s.hasAddr = true
}

func (s *webdynsunState) updateType(line string) {
	parts := strings.Split(line, ";")
	if len(parts) < 2 {
		return
	}
	s.typeInv = strings.TrimSpace(parts[1])
	s.hasType = true
}

func (s webdynsunState) inject(values map[string]any, opts TypedReadOptions) {
	if opts.WebdynsunAddressColumn != "" && s.hasAddr {
		values[opts.WebdynsunAddressColumn] = s.addr
	}
	if opts.WebdynsunTypeColumn != "" && s.hasType {
		values[opts.WebdynsunTypeColumn] = s.typeInv
	}
}

func isWebdynsunBlockHeader(raw []string) bool {
	if len(raw) == 0 {
		return false
	}
	first := strings.TrimSpace(raw[0])
	if first == "" {
		return false
	}
	for _, r := range first {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func openWebdynsunFile(path string) (io.ReadCloser, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(strings.ToLower(path), ".gz") {
		return file, nil
	}

	gz, err := gzip.NewReader(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	return &multiReadCloser{
		ReadCloser: gz,
		closers: []io.Closer{
			gz,
			file,
		},
	}, nil
}

type multiReadCloser struct {
	io.ReadCloser
	closers []io.Closer
}

func (m *multiReadCloser) Close() error {
	var firstErr error
	for _, closer := range m.closers {
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
