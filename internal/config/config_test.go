package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := `
inputs:
  - name: ac_power
    directory: /tmp/power
    mode: latest
    timestamp_column: TimeStamp
    delimiter: ";"
    decimal_comma: true
    include:
      - "*_ACpower.txt"
    storage:
      outputs:
        - name: local_timescale
          type: timescaledb
          enabled: true
          timescaledb:
            host_env: TIMESCALE_HOST_LOCAL
            port_env: TIMESCALE_PORT_LOCAL
            user_env: TIMESCALE_USER_LOCAL
            password_env: TIMESCALE_PASSWORD_LOCAL
            database_env: TIMESCALE_DB_LOCAL
            schema: ua
            table: cdaq2_ac_power
`

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := len(cfg.Inputs); got != 1 {
		t.Fatalf("len(Inputs) = %d, want 1", got)
	}
	if cfg.Inputs[0].Name != "ac_power" {
		t.Fatalf("Inputs[0].Name = %q, want %q", cfg.Inputs[0].Name, "ac_power")
	}
	if cfg.Inputs[0].TimestampColumn != "TimeStamp" {
		t.Fatalf("Inputs[0].TimestampColumn = %q, want %q", cfg.Inputs[0].TimestampColumn, "TimeStamp")
	}
	if cfg.Inputs[0].Mode != "latest" {
		t.Fatalf("Inputs[0].Mode = %q, want %q", cfg.Inputs[0].Mode, "latest")
	}
	if cfg.Inputs[0].Delimiter != ";" {
		t.Fatalf("Inputs[0].Delimiter = %q, want %q", cfg.Inputs[0].Delimiter, ";")
	}
	if !cfg.Inputs[0].DecimalComma {
		t.Fatalf("Inputs[0].DecimalComma = false, want true")
	}
	if got := len(cfg.Inputs[0].Storage.Outputs); got != 1 {
		t.Fatalf("len(Storage.Outputs) = %d, want 1", got)
	}
	if cfg.Inputs[0].Storage.Outputs[0].Type != "timescaledb" {
		t.Fatalf("Storage.Outputs[0].Type = %q, want %q", cfg.Inputs[0].Storage.Outputs[0].Type, "timescaledb")
	}
	if cfg.Inputs[0].Storage.Outputs[0].TimescaleDB.BatchSize != 5000 {
		t.Fatalf("Storage.Outputs[0].TimescaleDB.BatchSize = %d, want 5000", cfg.Inputs[0].Storage.Outputs[0].TimescaleDB.BatchSize)
	}
}

func TestValidateRejectsMissingInclude(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Inputs: []Input{
			{
				Name:      "ac_power",
				Directory: "/tmp/power",
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() expected error, got nil")
	}
}

func TestValidateRejectsUnsupportedColumnType(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Inputs: []Input{
			{
				Name:      "ac_power",
				Directory: "/tmp/power",
				Include:   []string{"*_ACpower.txt"},
				Columns: map[string]ColumnConfig{
					"TimeStamp": {Type: "datetime64"},
				},
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() expected error, got nil")
	}
}

func TestValidateRejectsMissingTimestampColumn(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Inputs: []Input{
			{
				Name:      "ac_power",
				Directory: "/tmp/power",
				Include:   []string{"*_ACpower.txt"},
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() expected error, got nil")
	}
}

func TestValidateAllowsCombinedTimestampColumns(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Inputs: []Input{
			{
				Name:                   "het1fixed",
				Directory:              "/tmp/pvstand",
				TimestampColumn:        "ts",
				TimestampSourceColumns: []string{"Date", "time"},
				TimestampLayouts:       []string{"02.01.2006 15:04:05"},
				Include:                []string{"*.iud"},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsCombinedTimestampWithoutLayouts(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Inputs: []Input{
			{
				Name:                   "het1fixed",
				Directory:              "/tmp/pvstand",
				TimestampColumn:        "ts",
				TimestampSourceColumns: []string{"Date", "time"},
				Include:                []string{"*.iud"},
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() expected error, got nil")
	}
}

func TestValidateDefaultsModeToAll(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Inputs: []Input{
			{
				Name:            "ac_power",
				Directory:       "/tmp/power",
				TimestampColumn: "TimeStamp",
				Include:         []string{"*_ACpower.txt"},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if cfg.Inputs[0].Mode != "all" {
		t.Fatalf("Mode = %q, want %q", cfg.Inputs[0].Mode, "all")
	}
}

func TestValidateRejectsLastNFilesWithoutCount(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Inputs: []Input{
			{
				Name:            "ac_power",
				Directory:       "/tmp/power",
				Mode:            "last_n_files",
				TimestampColumn: "TimeStamp",
				Include:         []string{"*_ACpower.txt"},
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() expected error, got nil")
	}
}

func TestValidateDefaultsReplayLinesForGrowingFile(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Inputs: []Input{
			{
				Name:            "meteo_psda",
				Directory:       "/tmp/meteo",
				Mode:            "growing_file",
				TimestampColumn: "TIMESTAMP",
				Include:         []string{"*.dat"},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if cfg.Inputs[0].ReplayLines != 5 {
		t.Fatalf("ReplayLines = %d, want 5", cfg.Inputs[0].ReplayLines)
	}
}

func TestValidateRejectsNegativeReplayLinesForGrowingFile(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Inputs: []Input{
			{
				Name:            "meteo_psda",
				Directory:       "/tmp/meteo",
				Mode:            "growing_file",
				ReplayLines:     -1,
				TimestampColumn: "TIMESTAMP",
				Include:         []string{"*.dat"},
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() expected error, got nil")
	}
}

func TestValidateRejectsInvalidTimescaleOutput(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Inputs: []Input{
			{
				Name:            "ac_power",
				Directory:       "/tmp/power",
				TimestampColumn: "TimeStamp",
				Include:         []string{"*_ACpower.txt"},
				Storage: StorageConfig{
					Outputs: []Output{
						{
							Name:    "local_timescale",
							Type:    "timescaledb",
							Enabled: true,
							TimescaleDB: TimescaleDBOutputConfig{
								Host:     "127.0.0.1",
								Port:     5432,
								User:     "collector",
								Password: "secret",
								Database: "telemetry",
								Schema:   "ua",
							},
						},
					},
				},
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() expected error, got nil")
	}
}

func TestValidateAllowsCustomTimescaleBatchSize(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Inputs: []Input{
			{
				Name:            "ac_power",
				Directory:       "/tmp/power",
				TimestampColumn: "TimeStamp",
				Include:         []string{"*_ACpower.txt"},
				Storage: StorageConfig{
					Outputs: []Output{
						{
							Name:    "local_timescale",
							Type:    "timescaledb",
							Enabled: true,
							TimescaleDB: TimescaleDBOutputConfig{
								Host:       "127.0.0.1",
								Port:       5432,
								User:       "collector",
								Password:   "secret",
								Database:   "telemetry",
								Schema:     "ua",
								Table:      "cdaq2_ac_power",
								BatchSize:  250,
								OnConflict: "do_nothing",
							},
						},
					},
				},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if cfg.Inputs[0].Storage.Outputs[0].TimescaleDB.BatchSize != 250 {
		t.Fatalf("BatchSize = %d, want 250", cfg.Inputs[0].Storage.Outputs[0].TimescaleDB.BatchSize)
	}
}
