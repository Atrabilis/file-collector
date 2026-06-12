package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/atamostec/file-collector/internal/config"
	"github.com/atamostec/file-collector/internal/tabular"
	"github.com/atamostec/file-collector/internal/timescaledb"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "read":
		if err := runRead(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "read failed: %v\n", err)
			os.Exit(1)
		}
	case "write":
		if err := runWrite(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func runRead(args []string) error {
	fs := flag.NewFlagSet("read", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	path := fs.String("path", "", "file or directory to read")
	glob := fs.String("glob", "", "optional file glob for directories, e.g. '*power.txt'")
	configPath := fs.String("config", "", "optional YAML config path")
	inputName := fs.String("input", "", "input name from YAML config")
	maxPreviewRows := fs.Int("preview-rows", 3, "number of rows to preview per file")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*configPath) != "" {
		return runReadFromConfig(*configPath, *inputName, *maxPreviewRows)
	}

	if strings.TrimSpace(*path) == "" {
		return fmt.Errorf("--path is required when --config is not used")
	}

	files, err := resolveFiles(*path, *glob)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no files matched")
	}

	for _, filePath := range files {
		summary, err := tabular.ReadFileSummary(filePath, *maxPreviewRows)
		if err != nil {
			fmt.Printf("FILE %s\n", filePath)
			fmt.Printf("  error: %v\n\n", err)
			continue
		}

		fmt.Printf("FILE %s\n", filePath)
		fmt.Printf("  delimiter: %q\n", string(summary.Delimiter))
		fmt.Printf("  columns: %d\n", len(summary.Header))
		fmt.Printf("  rows: %d\n", summary.RowCount)
		fmt.Printf("  well_formed_rows: %d\n", summary.WellFormedRowCount)
		fmt.Printf("  malformed_rows: %d\n", summary.MalformedRowCount)
		fmt.Printf("  header: %s\n", strings.Join(summary.Header, " | "))

		for idx, row := range summary.PreviewRows {
			fmt.Printf("  preview[%d]: %s\n", idx, strings.Join(row, " | "))
		}
		for idx, row := range summary.MalformedRows {
			fmt.Printf("  malformed[%d]: line=%d expected=%d actual=%d raw=%q\n", idx, row.LineNumber, row.ExpectedColumns, row.ActualColumns, row.Raw)
		}
		fmt.Println()
	}

	return nil
}

func runWrite(args []string) error {
	fs := flag.NewFlagSet("write", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "YAML config path")
	inputName := fs.String("input", "", "input name from YAML config")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*configPath) == "" {
		return fmt.Errorf("--config is required")
	}
	if strings.TrimSpace(*inputName) == "" {
		return fmt.Errorf("--input is required")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	var input *config.Input
	for idx := range cfg.Inputs {
		if cfg.Inputs[idx].Name == *inputName {
			input = &cfg.Inputs[idx]
			break
		}
	}
	if input == nil {
		return fmt.Errorf("input %q not found in config", *inputName)
	}

	files, err := resolveFilesFromInput(*input)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("input %q matched no files", input.Name)
	}

	var output *config.Output
	for idx := range input.Storage.Outputs {
		candidate := &input.Storage.Outputs[idx]
		if candidate.Enabled && candidate.Type == "timescaledb" {
			output = candidate
			break
		}
	}
	if output == nil {
		return fmt.Errorf("input %q has no enabled timescaledb output", input.Name)
	}

	fmt.Printf("INPUT %s\n", input.Name)
	fmt.Printf("  matched files: %d\n", len(files))
	fmt.Printf("  output: %s (%s.%s)\n", output.Name, output.TimescaleDB.Schema, output.TimescaleDB.Table)

	resolved, err := timescaledb.ResolveOutput(*output)
	if err != nil {
		return err
	}
	fmt.Printf("  dsn: %s\n", resolved.RedactedDSN())

	result, err := timescaledb.WriteInputFiles(context.Background(), *input, *output, files)
	if err != nil {
		return err
	}

	fmt.Printf("  rows_inserted: %d\n", result.RowsInserted)
	return nil
}

func runReadFromConfig(configPath, inputName string, maxPreviewRows int) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	inputs := cfg.Inputs
	if strings.TrimSpace(inputName) != "" {
		filtered := make([]config.Input, 0, 1)
		for _, input := range cfg.Inputs {
			if input.Name == inputName {
				filtered = append(filtered, input)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("input %q not found in config", inputName)
		}
		inputs = filtered
	}

	for _, input := range inputs {
		fmt.Printf("INPUT %s\n", input.Name)
		fmt.Printf("  directory: %s\n", input.Directory)
		fmt.Printf("  mode: %s\n", input.Mode)
		fmt.Printf("  timestamp_column: %s\n", input.TimestampColumn)
		if input.Delimiter != "" {
			fmt.Printf("  delimiter: %q\n", input.Delimiter)
		}
		fmt.Printf("  decimal_comma: %t\n", input.DecimalComma)
		fmt.Printf("  include: %s\n", strings.Join(input.Include, ", "))
		if len(input.Exclude) > 0 {
			fmt.Printf("  exclude: %s\n", strings.Join(input.Exclude, ", "))
		}
		if len(input.Storage.Outputs) > 0 {
			for _, output := range input.Storage.Outputs {
				fmt.Printf("  output %s:\n", output.Name)
				fmt.Printf("    type: %s\n", output.Type)
				fmt.Printf("    enabled: %t\n", output.Enabled)
				if output.Type == "timescaledb" {
					fmt.Printf("    timescaledb: %s.%s\n", output.TimescaleDB.Schema, output.TimescaleDB.Table)
					resolved, err := timescaledb.ResolveOutput(output)
					if err != nil {
						fmt.Printf("    resolve error: %v\n", err)
					} else {
						fmt.Printf("    dsn: %s\n", resolved.RedactedDSN())
					}
				}
			}
		}

		files, err := resolveFilesFromInput(input)
		if err != nil {
			fmt.Printf("  error: %v\n\n", err)
			continue
		}
		if len(files) == 0 {
			fmt.Printf("  matched files: 0\n\n")
			continue
		}

		fmt.Printf("  matched files: %d\n\n", len(files))
		var delimiter rune
		if input.Delimiter != "" {
			delimiter = []rune(input.Delimiter)[0]
		}

		columnSpecs := make(map[string]tabular.ColumnSpec, len(input.Columns))
		for columnName, column := range input.Columns {
			columnSpecs[columnName] = tabular.ColumnSpec{
				Type: tabular.ColumnType(column.Type),
			}
		}

		var timestampLocation *time.Location
		if strings.TrimSpace(input.TimestampTimezone) != "" {
			loc, err := time.LoadLocation(input.TimestampTimezone)
			if err != nil {
				fmt.Printf("  error: load timestamp timezone %q: %v\n\n", input.TimestampTimezone, err)
				continue
			}
			timestampLocation = loc
		}

		for _, filePath := range files {
			summary, err := tabular.ReadTypedFileSummary(filePath, maxPreviewRows, tabular.TypedReadOptions{
				Delimiter:              delimiter,
				DecimalComma:           input.DecimalComma,
				SkipLines:              input.SkipLines,
				HeaderColumns:          append([]string(nil), input.HeaderColumns...),
				TimestampColumn:        input.TimestampColumn,
				TimestampSourceColumns: append([]string(nil), input.TimestampSourceColumns...),
				ColumnSpecs:            columnSpecs,
				TimestampLayouts:       []string{"2006_01_02 15:04:05", time.RFC3339},
				TimestampLocation:      timestampLocation,
			})
			if err != nil {
				fmt.Printf("FILE %s\n", filePath)
				fmt.Printf("  error: %v\n\n", err)
				continue
			}

			fmt.Printf("FILE %s\n", filePath)
			fmt.Printf("  delimiter: %q\n", string(summary.Delimiter))
			fmt.Printf("  columns: %d\n", len(summary.Header))
			fmt.Printf("  rows: %d\n", summary.RowCount)
			fmt.Printf("  well_formed_rows: %d\n", summary.WellFormedRowCount)
			fmt.Printf("  malformed_rows: %d\n", summary.MalformedRowCount)
			fmt.Printf("  parse_errors: %d\n", summary.ParseErrorCount)
			fmt.Printf("  header: %s\n", strings.Join(summary.Header, " | "))
			for idx, row := range summary.PreviewRows {
				fmt.Printf("  preview[%d].raw: %s\n", idx, strings.Join(row.Raw, " | "))
				fmt.Printf("  preview[%d].typed: %#v\n", idx, row.Values)
			}
			for idx, row := range summary.MalformedRows {
				fmt.Printf("  malformed[%d]: line=%d expected=%d actual=%d raw=%q\n", idx, row.LineNumber, row.ExpectedColumns, row.ActualColumns, row.Raw)
			}
			for idx, row := range summary.ParseErrors {
				fmt.Printf("  parse_error[%d]: line=%d column=%q expected=%q raw=%q error=%q\n", idx, row.LineNumber, row.ColumnName, row.ExpectedType, row.Raw, row.Error)
			}
			fmt.Println()
		}
	}

	return nil
}

func resolveFiles(path, glob string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return []string{path}, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if glob != "" {
			matched, err := filepath.Match(glob, name)
			if err != nil {
				return nil, err
			}
			if !matched {
				continue
			}
		}

		files = append(files, filepath.Join(path, name))
	}

	return files, nil
}

func resolveFilesFromInput(input config.Input) ([]string, error) {
	entries, err := os.ReadDir(input.Directory)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !matchesAnyGlob(name, input.Include) {
			continue
		}
		if matchesAnyGlob(name, input.Exclude) {
			continue
		}

		files = append(files, filepath.Join(input.Directory, name))
	}

	if len(files) > 1 {
		sortFilesByModTimeDesc(files)
	}

	if input.Mode == "latest" && len(files) > 1 {
		return files[:1], nil
	}
	if input.Mode == "last_n_files" && len(files) > input.LastNFiles {
		return files[:input.LastNFiles], nil
	}

	return files, nil
}

func sortFilesByModTimeDesc(files []string) {
	sort.Slice(files, func(i, j int) bool {
		leftInfo, leftErr := os.Stat(files[i])
		rightInfo, rightErr := os.Stat(files[j])
		if leftErr != nil || rightErr != nil {
			return files[i] > files[j]
		}
		if leftInfo.ModTime().Equal(rightInfo.ModTime()) {
			return files[i] > files[j]
		}
		return leftInfo.ModTime().After(rightInfo.ModTime())
	})
}

func matchesAnyGlob(name string, globs []string) bool {
	if len(globs) == 0 {
		return false
	}

	for _, glob := range globs {
		matched, err := filepath.Match(glob, name)
		if err != nil {
			continue
		}
		if matched {
			return true
		}
	}

	return false
}

func printUsage() {
	fmt.Println("file-collector")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  file-collector read --path <file-or-dir> [--glob <pattern>] [--preview-rows <n>]")
	fmt.Println("  file-collector read --config <config.yml> [--input <name>] [--preview-rows <n>]")
	fmt.Println("  file-collector write --config <config.yml> --input <name>")
}
