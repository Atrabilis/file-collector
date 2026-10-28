package main

// End-to-end tests: build the real binary and run "write" the way the systemd
// timer does, against files in a temporary directory and a real PostgreSQL,
// then check the rows. Each case follows a production config (radiometry
// growing file, PVStand daily files, WebdynSun gzip files).
//
// They need a PostgreSQL server and are skipped unless FILE_COLLECTOR_E2E_PG is
// set to a connection URL, e.g. postgres://postgres:postgres@127.0.0.1:5432/postgres
// (CI provides one as a service container). If FILE_COLLECTOR_E2E_COVERDIR is
// set, the binary is built with -cover and writes its coverage data there.

import (
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atamostec/file-collector/internal/identifier"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type e2eEnv struct {
	db     *sql.DB
	bin    string
	schema string
	env    []string
}

func setupE2E(t *testing.T) *e2eEnv {
	t.Helper()
	dsn := os.Getenv("FILE_COLLECTOR_E2E_PG")
	if dsn == "" {
		t.Skip("FILE_COLLECTOR_E2E_PG not set; skipping end-to-end test")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse FILE_COLLECTOR_E2E_PG: %v", err)
	}
	password, _ := u.User.Password()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := fmt.Sprintf("e2e_%d", time.Now().UnixNano())
	mustExec(t, db, "CREATE SCHEMA "+schema)
	t.Cleanup(func() { _, _ = db.Exec("DROP SCHEMA " + schema + " CASCADE") })

	bin := filepath.Join(t.TempDir(), "file-collector")
	args := []string{"build", "-o", bin}
	if os.Getenv("FILE_COLLECTOR_E2E_COVERDIR") != "" {
		args = append(args, "-cover", "-coverpkg=github.com/atamostec/file-collector/...")
	}
	if out, err := exec.Command("go", append(args, ".")...).CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	env := append(os.Environ(),
		"E2E_PG_HOST="+u.Hostname(),
		"E2E_PG_PORT="+u.Port(),
		"E2E_PG_USER="+u.User.Username(),
		"E2E_PG_PASSWORD="+password,
		"E2E_PG_DB="+strings.TrimPrefix(u.Path, "/"),
	)
	if dir := os.Getenv("FILE_COLLECTOR_E2E_COVERDIR"); dir != "" {
		env = append(env, "GOCOVERDIR="+dir)
	}
	return &e2eEnv{db: db, bin: bin, schema: schema, env: env}
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// createTable creates a table with the columns every production table has
// (ts primary key plus the collector-managed ones) and the given data columns,
// named the way the collector normalizes file headers.
func (e *e2eEnv) createTable(t *testing.T, table string, columns map[string]string) {
	t.Helper()
	defs := []string{
		"ts TIMESTAMPTZ PRIMARY KEY",
		"source_file TEXT",
		"source_line_number INTEGER",
		"flags JSONB",
		"ingested_at TIMESTAMPTZ NOT NULL DEFAULT now()",
	}
	for header, sqlType := range columns {
		defs = append(defs, identifier.QuoteIfNeeded(identifier.NormalizeColumnName(header))+" "+sqlType)
	}
	mustExec(t, e.db, fmt.Sprintf("CREATE TABLE %s.%s (%s)", e.schema, table, strings.Join(defs, ", ")))
}

func (e *e2eEnv) storageBlock(table, onConflict string) string {
	return fmt.Sprintf(`    storage:
      outputs:
        - name: local
          type: timescaledb
          enabled: true
          timescaledb:
            host_env: E2E_PG_HOST
            port_env: E2E_PG_PORT
            user_env: E2E_PG_USER
            password_env: E2E_PG_PASSWORD
            database_env: E2E_PG_DB
            schema: %s
            table: %s
            sslmode: disable
            on_conflict: %s
            batch_size: 5000
`, e.schema, table, onConflict)
}

func (e *e2eEnv) write(t *testing.T, configPath, input string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, e.bin, "write", "--config", configPath, "--input", input)
	cmd.Env = e.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("file-collector write failed: %v\n%s", err, out)
	}
	return string(out)
}

func (e *e2eEnv) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow("SELECT count(*) FROM " + e.schema + "." + table).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func writeFile(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if !mtime.IsZero() {
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
}

func appendFile(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

// Radiometry logger (CR1000X, TOA5 format) written to a file that keeps
// growing, as in psda meteo_6857: four header lines, quoted timestamp, NAN for
// a missing value, and a last line still being written.
func TestE2EGrowingFileResumesAndHoldsUnterminatedLine(t *testing.T) {
	e := setupE2E(t)
	e.createTable(t, "meteo", map[string]string{
		"RECORD": "BIGINT", "BattV_Min": "DOUBLE PRECISION", "GHI_Avg": "DOUBLE PRECISION", "DNI_Avg": "DOUBLE PRECISION",
		// PTemp_C_Avg is in the file but not in the table: it must be skipped.
	})

	dataDir, stateDir, promDir := t.TempDir(), t.TempDir(), t.TempDir()
	dataFile := filepath.Join(dataDir, "CRx_6857_Meteo.dat")
	header := strings.Join([]string{
		`"TOA5","CRx_6857","CR1000X","12345","CR1000X.Std.04","CPU:meteo.CR1X","1234","Meteo"`,
		`"TIMESTAMP","RECORD","BattV_Min","PTemp_C_Avg","GHI_Avg","DNI_Avg"`,
		`"TS","RN","Volts","Deg C","W/m^2","W/m^2"`,
		`"","","Min","Avg","Avg","Avg"`,
	}, "\n") + "\n"
	writeFile(t, dataFile, header+
		`"2026-10-27 10:00:00",1,12.51,25.1,800.5,900.25`+"\n"+
		`"2026-10-27 10:01:00",2,12.50,25.2,NAN,905.5`+"\n"+
		`"2026-10-27 10:02:00",3,12.49,25.3,820.0,910.5`+"\n"+
		`"2026-10-27 10:03:00",4,12.48,25.4,830.1,9`, // still being written
		time.Time{})

	configPath := filepath.Join(t.TempDir(), "meteo.yml")
	writeFile(t, configPath, fmt.Sprintf(`inputs:
  - name: e2e__meteo
    directory: %s
    mode: growing_file
    replay_lines: 2
    state_directory: %s
    skip_lines: 4
    header_columns: [TIMESTAMP, RECORD, BattV_Min, PTemp_C_Avg, GHI_Avg, DNI_Avg]
    timestamp_column: ts
    timestamp_source_columns: [TIMESTAMP]
    timestamp_layouts: ["2006-01-02 15:04:05"]
    delimiter: ","
    include: ["CRx_6857_Meteo.dat"]
    columns:
      RECORD: {type: int64}
      BattV_Min: {type: float64}
      PTemp_C_Avg: {type: float64}
      GHI_Avg: {type: float64}
      DNI_Avg: {type: float64}
    prometheus_textfile:
      enabled: true
      directory: %s
      file_name: meteo.prom
      metrics:
        - name: atamostec_radiometry_dni_avg_watts_per_square_meter
          help: Latest DNI average from file-collector.
          value_column: DNI_Avg
          labels: {plant: e2e}
          timestamp_metric_name: atamostec_radiometry_sample_timestamp_seconds
          timestamp_metric_help: Timestamp of the latest source sample.
`, dataDir, stateDir, promDir)+e.storageBlock("meteo", "do_update"), time.Time{})

	out := e.write(t, configPath, "e2e__meteo")
	if n := e.count(t, "meteo"); n != 3 {
		t.Fatalf("first run: expected 3 complete lines written, got %d\n%s", n, out)
	}
	var ghi sql.NullFloat64
	if err := e.db.QueryRow("SELECT ghi_avg FROM " + e.schema + ".meteo WHERE record = 2").Scan(&ghi); err != nil {
		t.Fatal(err)
	}
	if ghi.Valid {
		t.Fatalf("NAN from the logger must be stored as NULL, got %v", ghi.Float64)
	}
	if entries, _ := os.ReadDir(stateDir); len(entries) == 0 {
		t.Fatal("growing_file must persist its resume state")
	}

	// The logger finishes the line and writes one more.
	appendFile(t, dataFile, "20.75\n"+`"2026-10-27 10:04:00",5,12.47,25.5,840.0,930.0`+"\n")
	out = e.write(t, configPath, "e2e__meteo")
	if n := e.count(t, "meteo"); n != 5 {
		t.Fatalf("second run: expected 5 rows, got %d\n%s", n, out)
	}
	var dni float64
	var sourceFile string
	var sourceLine int
	if err := e.db.QueryRow("SELECT dni_avg, source_file, source_line_number FROM "+e.schema+".meteo WHERE record = 4").Scan(&dni, &sourceFile, &sourceLine); err != nil {
		t.Fatal(err)
	}
	if dni != 920.75 {
		t.Fatalf("the held line must be read complete (920.75), got %v", dni)
	}
	if !strings.HasSuffix(sourceFile, "CRx_6857_Meteo.dat") || sourceLine != 8 {
		t.Fatalf("unexpected provenance: source_file=%q source_line_number=%d", sourceFile, sourceLine)
	}

	// A run without changes leaves the table as it is.
	e.write(t, configPath, "e2e__meteo")
	if n := e.count(t, "meteo"); n != 5 {
		t.Fatalf("unchanged file: expected 5 rows, got %d", n)
	}

	prom, err := os.ReadFile(filepath.Join(promDir, "meteo.prom"))
	if err != nil {
		t.Fatalf("prometheus textfile: %v", err)
	}
	for _, want := range []string{
		`atamostec_radiometry_dni_avg_watts_per_square_meter{plant="e2e"} 930`,
		"atamostec_radiometry_sample_timestamp_seconds",
	} {
		if !strings.Contains(string(prom), want) {
			t.Fatalf("textfile missing %q:\n%s", want, prom)
		}
	}
}

// PVStand daily files (psda pvstand_*): last_n_files picks the newest files
// by mtime, the timestamp comes from two columns, and on_conflict do_nothing
// must not rewrite rows that already exist (the bug fixed on 2026-10-13).
func TestE2ELastNFilesDoNothingDoesNotRewrite(t *testing.T) {
	e := setupE2E(t)
	e.createTable(t, "pvstand", map[string]string{"Pmax": "DOUBLE PRECISION", "I@pmax": "DOUBLE PRECISION"})

	dir := t.TempDir()
	now := time.Now()
	day := func(d, pmax string) string {
		return "Date\ttime\tPmax\tI@pmax\n" +
			d + "\t10:00:00\t" + pmax + "\t8.1\n" +
			d + "\t10:05:00\t" + pmax + "\t8.2\n"
	}
	writeFile(t, filepath.Join(dir, "a.iud"), day("24.10.2026", "240.0"), now.Add(-72*time.Hour))
	writeFile(t, filepath.Join(dir, "b.iud"), day("25.10.2026", "250.0"), now.Add(-48*time.Hour))
	writeFile(t, filepath.Join(dir, "c.iud"), day("26.10.2026", "260.0"), now.Add(-24*time.Hour))
	writeFile(t, filepath.Join(dir, "notes.txt"), "not a data file\n", now)

	configPath := filepath.Join(t.TempDir(), "pvstand.yml")
	writeFile(t, configPath, fmt.Sprintf(`inputs:
  - name: e2e__pvstand
    directory: %s
    mode: last_n_files
    last_n_files: 2
    timestamp_column: ts
    timestamp_source_columns: [Date, time]
    timestamp_layouts: ["02.01.2006 15:04:05"]
    include: ["*.iud"]
    columns:
      "Pmax": {type: float64}
      "I@pmax": {type: float64}
`, dir)+e.storageBlock("pvstand", "do_nothing"), time.Time{})

	out := e.write(t, configPath, "e2e__pvstand")
	if n := e.count(t, "pvstand"); n != 4 {
		t.Fatalf("expected the 4 rows of the 2 newest files, got %d\n%s", n, out)
	}
	var oldRows int
	if err := e.db.QueryRow("SELECT count(*) FROM " + e.schema + ".pvstand WHERE ts::date = '2026-10-24'").Scan(&oldRows); err != nil {
		t.Fatal(err)
	}
	if oldRows != 0 {
		t.Fatalf("the oldest file is outside last_n_files and must not be read, got %d rows", oldRows)
	}

	var firstIngest time.Time
	if err := e.db.QueryRow("SELECT max(ingested_at) FROM " + e.schema + ".pvstand").Scan(&firstIngest); err != nil {
		t.Fatal(err)
	}

	// The newest file is rewritten with a different value: do_nothing keeps
	// the stored rows untouched, so neither the value nor ingested_at change.
	writeFile(t, filepath.Join(dir, "c.iud"), day("26.10.2026", "999.0"), now.Add(-24*time.Hour))
	e.write(t, configPath, "e2e__pvstand")

	var secondIngest time.Time
	var pmax float64
	if err := e.db.QueryRow("SELECT max(ingested_at), max(pmax) FROM "+e.schema+".pvstand").Scan(&secondIngest, &pmax); err != nil {
		t.Fatal(err)
	}
	if !secondIngest.Equal(firstIngest) || pmax != 260 {
		t.Fatalf("do_nothing rewrote rows: ingested_at %v -> %v, max pmax %v", firstIngest, secondIngest, pmax)
	}
	var current float64
	col := identifier.QuoteIfNeeded(identifier.NormalizeColumnName("I@pmax"))
	if err := e.db.QueryRow("SELECT " + col + " FROM " + e.schema + ".pvstand WHERE ts = '2026-10-26 10:05:00+00'").Scan(&current); err != nil {
		t.Fatalf("read column normalized from I@pmax: %v", err)
	}
	if current != 8.2 {
		t.Fatalf("I@pmax = %v, want 8.2", current)
	}
}

// WebdynSun inverter files (lalcktur webdynsun_*): gzip, one block per
// inverter whose address and type go into columns of every row.
func TestE2EWebdynsunGzip(t *testing.T) {
	e := setupE2E(t)
	e.createTable(t, "webdynsun", map[string]string{
		"addr": "BIGINT", "inverter_type": "TEXT", "value_a": "DOUBLE PRECISION", "value_b": "DOUBLE PRECISION",
	})

	dir := t.TempDir()
	content := strings.Join([]string{
		"SNINV;ADDR23;016",
		"TypeINV;C2-C3-Oeste_INV_GENERIC.ini",
		"2;value_a;value_b",
		"18/06/26-16:30:06;1.5;2.5",
		"SNINV;ADDR24;017",
		"TypeINV;C2-C3-Oeste_INV_GENERIC.ini",
		"2;value_a;value_b",
		"18/06/26-16:31:06;3.5;4.5",
		"",
	}, "\n")
	f, err := os.Create(filepath.Join(dir, "C2-C3-Oeste_INV_1_1_260618_163100.csv.gz"))
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	if _, err := gz.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	configPath := filepath.Join(t.TempDir(), "webdynsun.yml")
	writeFile(t, configPath, fmt.Sprintf(`inputs:
  - name: e2e__webdynsun
    directory: %s
    file_type: webdynsun
    mode: latest
    timestamp_column: ts
    timestamp_source_columns: [TIMESTAMP]
    timestamp_layouts: ["02/01/06-15:04:05"]
    delimiter: ";"
    include: ["C2-C3-Oeste_INV_*.csv.gz"]
    header_columns: [TIMESTAMP, value_a, value_b]
    webdynsun:
      address_column: addr
      type_column: inverter_type
    columns:
      addr: {type: int64}
      inverter_type: {type: string}
      value_a: {type: float64}
      value_b: {type: float64}
`, dir)+e.storageBlock("webdynsun", "do_update"), time.Time{})

	out := e.write(t, configPath, "e2e__webdynsun")
	rows, err := e.db.Query("SELECT addr, inverter_type, value_a, value_b FROM " + e.schema + ".webdynsun ORDER BY ts")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	type row struct {
		addr       int64
		typ        string
		valueA, vB float64
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.addr, &r.typ, &r.valueA, &r.vB); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	want := []row{{23, "C2-C3-Oeste_INV_GENERIC.ini", 1.5, 2.5}, {24, "C2-C3-Oeste_INV_GENERIC.ini", 3.5, 4.5}}
	if len(got) != len(want) {
		t.Fatalf("expected %d rows, got %d: %+v\n%s", len(want), len(got), got, out)
	}
	for i := range want {
		if got[i].addr != want[i].addr || got[i].typ != want[i].typ ||
			math.Abs(got[i].valueA-want[i].valueA) > 1e-9 || math.Abs(got[i].vB-want[i].vB) > 1e-9 {
			t.Fatalf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
