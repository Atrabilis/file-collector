package timescaledb

import (
	"os"
	"testing"

	"github.com/atamostec/file-collector/internal/config"
)

func TestResolveOutputFromStaticValues(t *testing.T) {
	t.Parallel()

	resolved, err := ResolveOutput(config.Output{
		Name:    "local_timescale",
		Type:    "timescaledb",
		Enabled: true,
		TimescaleDB: config.TimescaleDBOutputConfig{
			Host:     "127.0.0.1",
			Port:     5432,
			User:     "collector",
			Password: "secret",
			Database: "telemetry",
			Schema:   "ua",
			Table:    "cdaq_power",
			SSLMode:  "disable",
		},
	})
	if err != nil {
		t.Fatalf("ResolveOutput() error = %v", err)
	}

	if resolved.Host != "127.0.0.1" || resolved.Port != 5432 {
		t.Fatalf("resolved endpoint = %s:%d", resolved.Host, resolved.Port)
	}
	if got := resolved.RedactedDSN(); got != "postgres://collector:***@127.0.0.1:5432/telemetry?sslmode=disable" {
		t.Fatalf("RedactedDSN() = %q", got)
	}
}

func TestResolveOutputFromEnvironment(t *testing.T) {
	t.Setenv("TEST_TS_HOST", "10.0.0.1")
	t.Setenv("TEST_TS_PORT", "5432")
	t.Setenv("TEST_TS_USER", "collector")
	t.Setenv("TEST_TS_PASSWORD", "secret")
	t.Setenv("TEST_TS_DB", "telemetry")

	resolved, err := ResolveOutput(config.Output{
		Name:    "local_timescale",
		Type:    "timescaledb",
		Enabled: true,
		TimescaleDB: config.TimescaleDBOutputConfig{
			HostEnv:     "TEST_TS_HOST",
			PortEnv:     "TEST_TS_PORT",
			UserEnv:     "TEST_TS_USER",
			PasswordEnv: "TEST_TS_PASSWORD",
			DatabaseEnv: "TEST_TS_DB",
			Schema:      "ua",
			Table:       "cdaq_power",
		},
	})
	if err != nil {
		t.Fatalf("ResolveOutput() error = %v", err)
	}

	if resolved.Host != "10.0.0.1" || resolved.User != "collector" || resolved.Database != "telemetry" {
		t.Fatalf("resolved = %#v", resolved)
	}
	if resolved.SSLMode != "disable" {
		t.Fatalf("SSLMode = %q, want disable", resolved.SSLMode)
	}
}

func TestResolveOutputFailsOnMissingEnvironment(t *testing.T) {
	_ = os.Unsetenv("TEST_TS_MISSING")
	_, err := ResolveOutput(config.Output{
		Name: "local_timescale",
		Type: "timescaledb",
		TimescaleDB: config.TimescaleDBOutputConfig{
			HostEnv:     "TEST_TS_MISSING",
			Port:        5432,
			User:        "collector",
			Password:    "secret",
			Database:    "telemetry",
			Schema:      "ua",
			Table:       "cdaq_power",
			SSLMode:     "disable",
		},
	})
	if err == nil {
		t.Fatalf("ResolveOutput() expected error, got nil")
	}
}
