package timescaledb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/atamostec/file-collector/internal/config"
	"github.com/atamostec/file-collector/internal/identifier"
	"github.com/atamostec/file-collector/internal/tabular"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type WriteResult struct {
	RowsInserted int
}

func WriteInputFiles(ctx context.Context, input config.Input, output config.Output, files []string) (*WriteResult, error) {
	connCfg, err := ResolveOutput(output)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("pgx", connCfg.DSN())
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	columnSpecs := make(map[string]tabular.ColumnSpec, len(input.Columns))
	for columnName, column := range input.Columns {
		columnSpecs[columnName] = tabular.ColumnSpec{Type: tabular.ColumnType(column.Type)}
	}

	opts := tabular.TypedReadOptions{
		DecimalComma:     input.DecimalComma,
		TimestampColumn:  input.TimestampColumn,
		ColumnSpecs:      columnSpecs,
		TimestampLayouts: []string{"2006_01_02 15:04:05", time.RFC3339},
	}
	if input.Delimiter != "" {
		opts.Delimiter = []rune(input.Delimiter)[0]
	}

	result := &WriteResult{}
	if input.Mode == "all" && input.Concurrency > 1 && len(files) > 1 {
		return writeFilesConcurrently(ctx, db, connCfg, input, output, files, opts, result)
	}

	for _, filePath := range files {
		fmt.Printf("  file_started: %s\n", filepath.Base(filePath))
		fileStartedAt := time.Now()
		fileResult := &WriteResult{}
		if err := writeSingleFile(ctx, db, connCfg, input, output, filePath, opts, fileResult); err != nil {
			return nil, fmt.Errorf("write %s: %w", filePath, err)
		}
		result.RowsInserted += fileResult.RowsInserted
		fmt.Printf("  file_completed: %s rows_inserted=%d elapsed=%s\n", filepath.Base(filePath), fileResult.RowsInserted, time.Since(fileStartedAt).Round(time.Millisecond))
	}

	return result, nil
}

func writeFilesConcurrently(ctx context.Context, db *sql.DB, connCfg *ConnectionConfig, input config.Input, output config.Output, files []string, opts tabular.TypedReadOptions, result *WriteResult) (*WriteResult, error) {
	workerCount := input.Concurrency
	if workerCount > len(files) {
		workerCount = len(files)
	}

type writeOutcome struct {
		filePath     string
		rowsInserted int
		elapsed      time.Duration
		err          error
	}

	fileCh := make(chan string)
	outcomeCh := make(chan writeOutcome, len(files))

	var wg sync.WaitGroup
	var resultMu sync.Mutex

	for workerIdx := 0; workerIdx < workerCount; workerIdx++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range fileCh {
				fileStartedAt := time.Now()
				singleResult := &WriteResult{}
				err := writeSingleFile(ctx, db, connCfg, input, output, filePath, opts, singleResult)
				outcomeCh <- writeOutcome{
					filePath:     filePath,
					rowsInserted: singleResult.RowsInserted,
					elapsed:      time.Since(fileStartedAt),
					err:          err,
				}
			}
		}()
	}

	go func() {
		defer close(fileCh)
		for _, filePath := range files {
			fileCh <- filePath
		}
	}()

	go func() {
		wg.Wait()
		close(outcomeCh)
	}()

	var firstErr error
	for outcome := range outcomeCh {
		if outcome.err == nil {
			resultMu.Lock()
			result.RowsInserted += outcome.rowsInserted
			resultMu.Unlock()
			fmt.Printf("  file_completed: %s rows_inserted=%d elapsed=%s\n", filepath.Base(outcome.filePath), outcome.rowsInserted, outcome.elapsed.Round(time.Millisecond))
		}
		if outcome.err != nil && firstErr == nil {
			firstErr = fmt.Errorf("write %s: %w", outcome.filePath, outcome.err)
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}

	return result, nil
}

func writeSingleFile(ctx context.Context, db *sql.DB, connCfg *ConnectionConfig, input config.Input, output config.Output, filePath string, opts tabular.TypedReadOptions, result *WriteResult) error {
	var statement string
	var err error
	statementBuilt := false

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	err = tabular.ForEachTypedRow(filePath, opts, func(row tabular.TypedRow) error {
		tsValue, ok := row.Values[input.TimestampColumn]
		if !ok {
			return fmt.Errorf("missing timestamp column %q at line %d", input.TimestampColumn, row.LineNumber)
		}

		ts, ok := tsValue.(time.Time)
		if !ok {
			return fmt.Errorf("timestamp column %q is not parsed as time at line %d", input.TimestampColumn, row.LineNumber)
		}

		columns, values, err := buildInsertPayload(input, filePath, row, ts)
		if err != nil {
			return err
		}

		if !statementBuilt {
			statement = buildInsertStatement(connCfg.Schema, connCfg.Table, columns, output.TimescaleDB.OnConflict)
			statementBuilt = true
		}

		execResult, err := tx.ExecContext(ctx, statement, values...)
		if err != nil {
			return fmt.Errorf("exec insert at line %d: %w", row.LineNumber, err)
		}
		rowsAffected, err := execResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected at line %d: %w", row.LineNumber, err)
		}
		result.RowsInserted += int(rowsAffected)
		return nil
	})
	if err != nil {
		return err
	}

	return tx.Commit()
}

func buildInsertPayload(input config.Input, filePath string, row tabular.TypedRow, ts time.Time) ([]string, []any, error) {
	flagsJSON, err := json.Marshal(map[string]any{})
	if err != nil {
		return nil, nil, err
	}

	columns := []string{"ts", "source_file", "source_line_number", "flags"}
	values := []any{ts, filepath.Base(filePath), row.LineNumber, flagsJSON}

	for _, rawColumnName := range sortedKeys(row.Values) {
		if rawColumnName == input.TimestampColumn {
			continue
		}

		normalized := identifier.NormalizeColumnName(rawColumnName)
		if normalized == "" {
			continue
		}

		value := row.Values[rawColumnName]
		switch typed := value.(type) {
		case float64, int64, int, string, bool, nil:
			columns = append(columns, normalized)
			values = append(values, typed)
		case time.Time:
			columns = append(columns, normalized)
			values = append(values, typed)
		default:
			columns = append(columns, normalized)
			values = append(values, fmt.Sprint(typed))
		}
	}

	return columns, values, nil
}

func buildInsertStatement(schema, table string, columns []string, onConflict string) string {
	quotedColumns := make([]string, 0, len(columns))
	placeholders := make([]string, 0, len(columns))
	assignments := make([]string, 0, len(columns))

	for idx, column := range columns {
		quoted := identifier.QuoteIfNeeded(column)
		quotedColumns = append(quotedColumns, quoted)
		placeholders = append(placeholders, fmt.Sprintf("$%d", idx+1))
		if column == "ts" {
			continue
		}
		assignments = append(assignments, fmt.Sprintf("%s = EXCLUDED.%s", quoted, quoted))
	}

	conflictClause := `ON CONFLICT (ts) DO NOTHING`
	if onConflict == "do_update" {
		conflictClause = fmt.Sprintf("ON CONFLICT (ts) DO UPDATE SET %s", strings.Join(assignments, ", "))
	}

	return fmt.Sprintf(
		`INSERT INTO %s.%s (%s) VALUES (%s) %s`,
		schema,
		table,
		strings.Join(quotedColumns, ", "),
		strings.Join(placeholders, ", "),
		conflictClause,
	)
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
