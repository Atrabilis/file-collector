package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Input struct {
	Name            string                  `yaml:"name"`
	Directory       string                  `yaml:"directory"`
	Mode            string                  `yaml:"mode"`
	LastNFiles      int                     `yaml:"last_n_files"`
	Concurrency     int                     `yaml:"concurrency"`
	TimestampColumn string                  `yaml:"timestamp_column"`
	Delimiter       string                  `yaml:"delimiter"`
	DecimalComma    bool                    `yaml:"decimal_comma"`
	Include         []string                `yaml:"include"`
	Exclude         []string                `yaml:"exclude"`
	Columns         map[string]ColumnConfig `yaml:"columns"`
	Storage         StorageConfig           `yaml:"storage"`
}

type ColumnConfig struct {
	Type string `yaml:"type"`
}

type StorageConfig struct {
	Outputs []Output `yaml:"outputs"`
}

type Output struct {
	Name        string                  `yaml:"name"`
	Type        string                  `yaml:"type"`
	Enabled     bool                    `yaml:"enabled"`
	TimescaleDB TimescaleDBOutputConfig `yaml:"timescaledb"`
}

type TimescaleDBOutputConfig struct {
	Host        string `yaml:"host"`
	HostEnv     string `yaml:"host_env"`
	Port        int    `yaml:"port"`
	PortEnv     string `yaml:"port_env"`
	User        string `yaml:"user"`
	UserEnv     string `yaml:"user_env"`
	Password    string `yaml:"password"`
	PasswordEnv string `yaml:"password_env"`
	Database    string `yaml:"database"`
	DatabaseEnv string `yaml:"database_env"`
	Schema      string `yaml:"schema"`
	Table       string `yaml:"table"`
	SSLMode     string `yaml:"sslmode"`
	OnConflict  string `yaml:"on_conflict"`
	BatchSize   int    `yaml:"batch_size"`
}

type Config struct {
	Inputs []Input `yaml:"inputs"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if len(c.Inputs) == 0 {
		return fmt.Errorf("config must contain at least one input")
	}

	for idx, input := range c.Inputs {
		if strings.TrimSpace(input.Name) == "" {
			return fmt.Errorf("input %d has empty name", idx)
		}
		if strings.TrimSpace(input.Directory) == "" {
			return fmt.Errorf("input %q has empty directory", input.Name)
		}
		if strings.TrimSpace(input.Mode) == "" {
			input.Mode = "all"
			c.Inputs[idx].Mode = "all"
		}
		if !isSupportedMode(input.Mode) {
			return fmt.Errorf("input %q has unsupported mode %q", input.Name, input.Mode)
		}
		if input.Mode == "last_n_files" && input.LastNFiles <= 0 {
			return fmt.Errorf("input %q with mode %q requires last_n_files > 0", input.Name, input.Mode)
		}
		if input.Concurrency <= 0 {
			c.Inputs[idx].Concurrency = 1
			input.Concurrency = 1
		}
		if strings.TrimSpace(input.TimestampColumn) == "" {
			return fmt.Errorf("input %q has empty timestamp_column", input.Name)
		}
		if input.Delimiter != "" && len([]rune(input.Delimiter)) != 1 {
			return fmt.Errorf("input %q delimiter must be a single character", input.Name)
		}
		if len(input.Include) == 0 {
			return fmt.Errorf("input %q must define at least one include glob", input.Name)
		}
		for columnName, column := range input.Columns {
			if strings.TrimSpace(columnName) == "" {
				return fmt.Errorf("input %q has empty column name override", input.Name)
			}
			if !isSupportedColumnType(column.Type) {
				return fmt.Errorf("input %q column %q has unsupported type %q", input.Name, columnName, column.Type)
			}
		}
		for outputIdx, output := range input.Storage.Outputs {
			if strings.TrimSpace(output.Name) == "" {
				return fmt.Errorf("input %q output %d has empty name", input.Name, outputIdx)
			}
			if strings.TrimSpace(output.Type) == "" {
				return fmt.Errorf("input %q output %q has empty type", input.Name, output.Name)
			}
			if !isSupportedOutputType(output.Type) {
				return fmt.Errorf("input %q output %q has unsupported type %q", input.Name, output.Name, output.Type)
			}
			if strings.TrimSpace(output.TimescaleDB.SSLMode) == "" {
				c.Inputs[idx].Storage.Outputs[outputIdx].TimescaleDB.SSLMode = "disable"
				output.TimescaleDB.SSLMode = "disable"
			}
			if strings.TrimSpace(output.TimescaleDB.OnConflict) == "" {
				c.Inputs[idx].Storage.Outputs[outputIdx].TimescaleDB.OnConflict = "do_update"
				output.TimescaleDB.OnConflict = "do_update"
			}
			if output.TimescaleDB.BatchSize <= 0 {
				c.Inputs[idx].Storage.Outputs[outputIdx].TimescaleDB.BatchSize = 5000
				output.TimescaleDB.BatchSize = 5000
			}
			if output.Type == "timescaledb" {
				if err := validateTimescaleDBOutput(input.Name, output); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func validateTimescaleDBOutput(inputName string, output Output) error {
	cfg := output.TimescaleDB
	if strings.TrimSpace(cfg.Host) == "" && strings.TrimSpace(cfg.HostEnv) == "" {
		return fmt.Errorf("input %q output %q timescaledb requires host or host_env", inputName, output.Name)
	}
	if cfg.Port == 0 && strings.TrimSpace(cfg.PortEnv) == "" {
		return fmt.Errorf("input %q output %q timescaledb requires port or port_env", inputName, output.Name)
	}
	if strings.TrimSpace(cfg.User) == "" && strings.TrimSpace(cfg.UserEnv) == "" {
		return fmt.Errorf("input %q output %q timescaledb requires user or user_env", inputName, output.Name)
	}
	if strings.TrimSpace(cfg.Password) == "" && strings.TrimSpace(cfg.PasswordEnv) == "" {
		return fmt.Errorf("input %q output %q timescaledb requires password or password_env", inputName, output.Name)
	}
	if strings.TrimSpace(cfg.Database) == "" && strings.TrimSpace(cfg.DatabaseEnv) == "" {
		return fmt.Errorf("input %q output %q timescaledb requires database or database_env", inputName, output.Name)
	}
	if strings.TrimSpace(cfg.Schema) == "" {
		return fmt.Errorf("input %q output %q timescaledb requires schema", inputName, output.Name)
	}
	if strings.TrimSpace(cfg.Table) == "" {
		return fmt.Errorf("input %q output %q timescaledb requires table", inputName, output.Name)
	}
	if !isSupportedOnConflict(cfg.OnConflict) {
		return fmt.Errorf("input %q output %q timescaledb has unsupported on_conflict %q", inputName, output.Name, cfg.OnConflict)
	}
	if cfg.BatchSize <= 0 {
		return fmt.Errorf("input %q output %q timescaledb requires batch_size > 0", inputName, output.Name)
	}
	return nil
}

func isSupportedColumnType(value string) bool {
	switch strings.TrimSpace(value) {
	case "timestamp", "float64", "int64", "string", "bool":
		return true
	default:
		return false
	}
}

func isSupportedMode(value string) bool {
	switch strings.TrimSpace(value) {
	case "all", "latest", "last_n_files":
		return true
	default:
		return false
	}
}

func isSupportedOutputType(value string) bool {
	switch strings.TrimSpace(value) {
	case "timescaledb":
		return true
	default:
		return false
	}
}

func isSupportedOnConflict(value string) bool {
	switch strings.TrimSpace(value) {
	case "do_update", "do_nothing":
		return true
	default:
		return false
	}
}
