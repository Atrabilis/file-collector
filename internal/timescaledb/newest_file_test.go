package timescaledb

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewestFilePicksMostRecentModTime(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	older := filepath.Join(dir, "climate_20261012.vct")
	newer := filepath.Join(dir, "climate_20261013.vct")
	for _, p := range []string{older, newer} {
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", p, err)
		}
	}
	now := time.Now()
	if err := os.Chtimes(older, now.Add(-24*time.Hour), now.Add(-24*time.Hour)); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}
	if err := os.Chtimes(newer, now, now); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	if got := newestFile([]string{older, newer}); got != newer {
		t.Fatalf("newestFile() = %q, want %q", got, newer)
	}
	if got := newestFile([]string{filepath.Join(dir, "missing.txt")}); got != "" {
		t.Fatalf("newestFile(missing) = %q, want empty", got)
	}
}
