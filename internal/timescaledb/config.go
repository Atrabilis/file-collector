package timescaledb

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/atamostec/file-collector/internal/config"
)

type ConnectionConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	Schema   string
	Table    string
	SSLMode  string
}

func ResolveOutput(output config.Output) (*ConnectionConfig, error) {
	if output.Type != "timescaledb" {
		return nil, fmt.Errorf("unsupported output type %q", output.Type)
	}

	host, err := firstNonEmpty(output.TimescaleDB.Host, output.TimescaleDB.HostEnv)
	if err != nil {
		return nil, fmt.Errorf("resolve host: %w", err)
	}
	user, err := firstNonEmpty(output.TimescaleDB.User, output.TimescaleDB.UserEnv)
	if err != nil {
		return nil, fmt.Errorf("resolve user: %w", err)
	}
	password, err := firstNonEmpty(output.TimescaleDB.Password, output.TimescaleDB.PasswordEnv)
	if err != nil {
		return nil, fmt.Errorf("resolve password: %w", err)
	}
	database, err := firstNonEmpty(output.TimescaleDB.Database, output.TimescaleDB.DatabaseEnv)
	if err != nil {
		return nil, fmt.Errorf("resolve database: %w", err)
	}
	port, err := resolvePort(output.TimescaleDB.Port, output.TimescaleDB.PortEnv)
	if err != nil {
		return nil, fmt.Errorf("resolve port: %w", err)
	}

	sslmode := strings.TrimSpace(output.TimescaleDB.SSLMode)
	if sslmode == "" {
		sslmode = "disable"
	}

	return &ConnectionConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		Database: database,
		Schema:   output.TimescaleDB.Schema,
		Table:    output.TimescaleDB.Table,
		SSLMode:  sslmode,
	}, nil
}

func (c ConnectionConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(c.User),
		url.QueryEscape(c.Password),
		c.Host,
		c.Port,
		url.PathEscape(c.Database),
		url.QueryEscape(c.SSLMode),
	)
}

func (c ConnectionConfig) RedactedDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(c.User),
		"***",
		c.Host,
		c.Port,
		url.PathEscape(c.Database),
		url.QueryEscape(c.SSLMode),
	)
}

func firstNonEmpty(value, envName string) (string, error) {
	if strings.TrimSpace(value) != "" {
		return value, nil
	}
	if strings.TrimSpace(envName) == "" {
		return "", fmt.Errorf("no value or env configured")
	}
	resolved := strings.TrimSpace(os.Getenv(envName))
	if resolved == "" {
		return "", fmt.Errorf("environment variable %q is empty", envName)
	}
	return resolved, nil
}

func resolvePort(value int, envName string) (int, error) {
	if value > 0 {
		return value, nil
	}
	if strings.TrimSpace(envName) == "" {
		return 0, fmt.Errorf("no value or env configured")
	}
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return 0, fmt.Errorf("environment variable %q is empty", envName)
	}
	port, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid port in %q: %w", envName, err)
	}
	if port <= 0 {
		return 0, fmt.Errorf("port from %q must be positive", envName)
	}
	return port, nil
}
