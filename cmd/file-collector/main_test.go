package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/atamostec/file-collector/internal/config"
)

func TestResolveFilesFromInputAllReturnsAllMatches(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	older := filepath.Join(dir, "20260603_ACpower.txt")
	newer := filepath.Join(dir, "20260604_ACpower.txt")
	mustWriteTestFile(t, older)
	mustWriteTestFile(t, newer)
	mustWriteTestFile(t, filepath.Join(dir, "20260604_DCpower.txt"))

	now := time.Now()
	if err := os.Chtimes(older, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("Chtimes(%q) error = %v", older, err)
	}
	if err := os.Chtimes(newer, now, now); err != nil {
		t.Fatalf("Chtimes(%q) error = %v", newer, err)
	}

	files, err := resolveFilesFromInput(config.Input{
		Name:            "ac_power",
		Directory:       dir,
		Mode:            "all",
		TimestampColumn: "TimeStamp",
		Include:         []string{"*_ACpower.txt"},
	})
	if err != nil {
		t.Fatalf("resolveFilesFromInput() error = %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(files))
	}

	want := []string{
		newer,
		older,
	}
	for idx := range want {
		if files[idx] != want[idx] {
			t.Fatalf("files[%d] = %q, want %q", idx, files[idx], want[idx])
		}
	}
}

func TestResolveFilesFromInputLatestReturnsMostRecentlyModifiedMatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	older := filepath.Join(dir, "20260603_ACpower.txt")
	newer := filepath.Join(dir, "20260604_ACpower.txt")
	mustWriteTestFile(t, older)
	mustWriteTestFile(t, newer)

	now := time.Now()
	if err := os.Chtimes(older, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("Chtimes(%q) error = %v", older, err)
	}
	if err := os.Chtimes(newer, now, now); err != nil {
		t.Fatalf("Chtimes(%q) error = %v", newer, err)
	}

	files, err := resolveFilesFromInput(config.Input{
		Name:            "ac_power",
		Directory:       dir,
		Mode:            "latest",
		TimestampColumn: "TimeStamp",
		Include:         []string{"*_ACpower.txt"},
	})
	if err != nil {
		t.Fatalf("resolveFilesFromInput() error = %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	if files[0] != newer {
		t.Fatalf("files[0] = %q, want %q", files[0], newer)
	}
}

func TestResolveFilesFromInputLastNFilesReturnsMostRecentMatches(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	oldest := filepath.Join(dir, "20260602_ACpower.txt")
	middle := filepath.Join(dir, "20260603_ACpower.txt")
	newest := filepath.Join(dir, "20260604_ACpower.txt")
	mustWriteTestFile(t, oldest)
	mustWriteTestFile(t, middle)
	mustWriteTestFile(t, newest)

	now := time.Now()
	if err := os.Chtimes(oldest, now.Add(-3*time.Hour), now.Add(-3*time.Hour)); err != nil {
		t.Fatalf("Chtimes(%q) error = %v", oldest, err)
	}
	if err := os.Chtimes(middle, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("Chtimes(%q) error = %v", middle, err)
	}
	if err := os.Chtimes(newest, now.Add(-1*time.Hour), now.Add(-1*time.Hour)); err != nil {
		t.Fatalf("Chtimes(%q) error = %v", newest, err)
	}

	files, err := resolveFilesFromInput(config.Input{
		Name:            "ac_power",
		Directory:       dir,
		Mode:            "last_n_files",
		LastNFiles:      2,
		TimestampColumn: "TimeStamp",
		Include:         []string{"*_ACpower.txt"},
	})
	if err != nil {
		t.Fatalf("resolveFilesFromInput() error = %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(files))
	}
	if files[0] != newest {
		t.Fatalf("files[0] = %q, want %q", files[0], newest)
	}
	if files[1] != middle {
		t.Fatalf("files[1] = %q, want %q", files[1], middle)
	}
}

func mustWriteTestFile(t *testing.T, path string) {
	t.Helper()

	content := "TimeStamp;Value\n2026_06_04 00:00:03;49,91\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
