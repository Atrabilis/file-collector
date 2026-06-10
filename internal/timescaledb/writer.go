package timescaledb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
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

type fileWriteStats struct {
	sourceColumns             map[string]struct{}
	skippedDestinationColumns map[string]struct{}
	nanToNullCount            int
}

const maxPostgresParameters = 65535

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

	destinationColumns, err := loadDestinationColumns(ctx, db, connCfg)
	if err != nil {
		return nil, err
	}

	columnSpecs := make(map[string]tabular.ColumnSpec, len(input.Columns))
	for columnName, column := range input.Columns {
		columnSpecs[columnName] = tabular.ColumnSpec{Type: tabular.ColumnType(column.Type)}
	}

	opts := tabular.TypedReadOptions{
		DecimalComma:           input.DecimalComma,
		TimestampColumn:        input.TimestampColumn,
		TimestampSourceColumns: append([]string(nil), input.TimestampSourceColumns...),
		ColumnSpecs:            columnSpecs,
		TimestampLayouts:       []string{"2006_01_02 15:04:05", time.RFC3339},
	}
	if len(input.TimestampLayouts) > 0 {
		opts.TimestampLayouts = append([]string(nil), input.TimestampLayouts...)
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
		if err := writeSingleFile(ctx, db, connCfg, input, output, filePath, opts, destinationColumns, fileResult); err != nil {
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
				destinationColumns, err := loadDestinationColumns(ctx, db, connCfg)
				if err == nil {
					err = writeSingleFile(ctx, db, connCfg, input, output, filePath, opts, destinationColumns, singleResult)
				}
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

func writeSingleFile(ctx context.Context, db *sql.DB, connCfg *ConnectionConfig, input config.Input, output config.Output, filePath string, opts tabular.TypedReadOptions, destinationColumns map[string]struct{}, result *WriteResult) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var batchColumns []string
	var batchValues []any
	batchRowCount := 0
	batchLastLineNumber := 0
	stats := &fileWriteStats{
		sourceColumns:             make(map[string]struct{}),
		skippedDestinationColumns: make(map[string]struct{}),
	}

	flushBatch := func() error {
		if batchRowCount == 0 {
			return nil
		}

		statement := buildInsertStatement(connCfg.Schema, connCfg.Table, batchColumns, batchRowCount, output.TimescaleDB.OnConflict)
		execResult, err := tx.ExecContext(ctx, statement, batchValues...)
		if err != nil {
			return fmt.Errorf("exec insert batch ending at line %d: %w", batchLastLineNumber, err)
		}
		rowsAffected, err := execResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected for insert batch: %w", err)
		}
		result.RowsInserted += int(rowsAffected)

		batchColumns = nil
		batchValues = nil
		batchRowCount = 0
		batchLastLineNumber = 0
		return nil
	}

	err = tabular.ForEachTypedRow(filePath, opts, func(row tabular.TypedRow) error {
		tsValue, ok := row.Values[input.TimestampColumn]
		if !ok {
			return fmt.Errorf("missing timestamp column %q at line %d", input.TimestampColumn, row.LineNumber)
		}

		ts, ok := tsValue.(time.Time)
		if !ok {
			return fmt.Errorf("timestamp column %q is not parsed as time at line %d", input.TimestampColumn, row.LineNumber)
		}

		columns, values, err := buildInsertPayload(input, filePath, row, ts, destinationColumns, stats)
		if err != nil {
			return err
		}

		if batchRowCount > 0 && !sameColumns(batchColumns, columns) {
			if err := flushBatch(); err != nil {
				return err
			}
		}

		if batchRowCount == 0 {
			batchColumns = append([]string(nil), columns...)
		}

		maxBatchRows := effectiveBatchSize(output.TimescaleDB.BatchSize, len(batchColumns))
		if maxBatchRows <= 0 {
			return fmt.Errorf("no valid batch size for %d columns", len(batchColumns))
		}

		batchValues = append(batchValues, values...)
		batchRowCount++
		batchLastLineNumber = row.LineNumber

		if batchRowCount >= maxBatchRows {
			if err := flushBatch(); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	if err := flushBatch(); err != nil {
		return err
	}

	logFileWriteStats(filePath, destinationColumns, stats)

	return tx.Commit()
}

func loadDestinationColumns(ctx context.Context, db *sql.DB, connCfg *ConnectionConfig) (map[string]struct{}, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT column_name
		   FROM information_schema.columns
		  WHERE table_schema = $1
		    AND table_name = $2`,
		connCfg.Schema,
		connCfg.Table,
	)
	if err != nil {
		return nil, fmt.Errorf("load destination columns for %s.%s: %w", connCfg.Schema, connCfg.Table, err)
	}
	defer rows.Close()

	columns := make(map[string]struct{})
	for rows.Next() {
		var columnName string
		if err := rows.Scan(&columnName); err != nil {
			return nil, fmt.Errorf("scan destination column for %s.%s: %w", connCfg.Schema, connCfg.Table, err)
		}
		columns[columnName] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate destination columns for %s.%s: %w", connCfg.Schema, connCfg.Table, err)
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("destination table %s.%s has no visible columns", connCfg.Schema, connCfg.Table)
	}
	return columns, nil
}

func buildInsertPayload(input config.Input, filePath string, row tabular.TypedRow, ts time.Time, destinationColumns map[string]struct{}, stats *fileWriteStats) ([]string, []any, error) {
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
		if containsString(input.TimestampSourceColumns, rawColumnName) {
			continue
		}

		normalized := identifier.NormalizeColumnName(rawColumnName)
		if normalized == "" {
			continue
		}
		if stats != nil {
			stats.sourceColumns[normalized] = struct{}{}
		}
		if _, ok := destinationColumns[normalized]; !ok {
			if stats != nil {
				stats.skippedDestinationColumns[normalized] = struct{}{}
			}
			continue
		}

		value := row.Values[rawColumnName]
		switch typed := value.(type) {
		case float64:
			if math.IsNaN(typed) {
				columns = append(columns, normalized)
				values = append(values, nil)
				if stats != nil {
					stats.nanToNullCount++
				}
				continue
			}
			columns = append(columns, normalized)
			values = append(values, typed)
		case int64, int, string, bool, nil:
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

func logFileWriteStats(filePath string, destinationColumns map[string]struct{}, stats *fileWriteStats) {
	if stats == nil {
		return
	}

	skipped := sortedMapKeys(stats.skippedDestinationColumns)
	if len(skipped) > 0 {
		fmt.Printf("  file_info: %s skipped_destination_missing_columns=%s\n", filepath.Base(filePath), strings.Join(skipped, ","))
	}

	missingFromSource := make([]string, 0)
	for column := range destinationColumns {
		if isCollectorManagedColumn(column) {
			continue
		}
		if _, ok := stats.sourceColumns[column]; ok {
			continue
		}
		missingFromSource = append(missingFromSource, column)
	}
	sort.Strings(missingFromSource)
	if len(missingFromSource) > 0 {
		fmt.Printf("  file_info: %s destination_columns_missing_in_source=%s\n", filepath.Base(filePath), strings.Join(missingFromSource, ","))
	}

	if stats.nanToNullCount > 0 {
		fmt.Printf("  file_info: %s nan_to_null_count=%d\n", filepath.Base(filePath), stats.nanToNullCount)
	}
}

func sortedMapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isCollectorManagedColumn(column string) bool {
	switch column {
	case "ts", "source_file", "source_line_number", "flags", "ingested_at":
		return true
	default:
		return false
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func buildInsertStatement(schema, table string, columns []string, rowCount int, onConflict string) string {
	quotedColumns := make([]string, 0, len(columns))
	valueGroups := make([]string, 0, rowCount)
	assignments := make([]string, 0, len(columns))

	for _, column := range columns {
		quoted := identifier.QuoteIfNeeded(column)
		quotedColumns = append(quotedColumns, quoted)
		if column == "ts" {
			continue
		}
		assignments = append(assignments, fmt.Sprintf("%s = EXCLUDED.%s", quoted, quoted))
	}

	for rowIdx := 0; rowIdx < rowCount; rowIdx++ {
		placeholders := make([]string, 0, len(columns))
		for colIdx := range columns {
			placeholderIdx := rowIdx*len(columns) + colIdx + 1
			placeholders = append(placeholders, fmt.Sprintf("$%d", placeholderIdx))
		}
		valueGroups = append(valueGroups, fmt.Sprintf("(%s)", strings.Join(placeholders, ", ")))
	}

	conflictClause := `ON CONFLICT (ts) DO NOTHING`
	if onConflict == "do_update" {
		conflictClause = fmt.Sprintf("ON CONFLICT (ts) DO UPDATE SET %s", strings.Join(assignments, ", "))
	}

	return fmt.Sprintf(
		`INSERT INTO %s.%s (%s) VALUES %s %s`,
		schema,
		table,
		strings.Join(quotedColumns, ", "),
		strings.Join(valueGroups, ", "),
		conflictClause,
	)
}

func effectiveBatchSize(configuredBatchSize, columnsPerRow int) int {
	if configuredBatchSize <= 0 || columnsPerRow <= 0 {
		return 0
	}

	maxRowsByParameters := maxPostgresParameters / columnsPerRow
	if maxRowsByParameters <= 0 {
		return 1
	}
	if configuredBatchSize < maxRowsByParameters {
		return configuredBatchSize
	}
	return maxRowsByParameters
}

func sameColumns(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
