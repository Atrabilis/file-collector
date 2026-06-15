package timescaledb

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildResumePlanSkipsUnchangedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "meteo.dat")
	if err := os.WriteFile(path, []byte("header\nrow\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	state := &fileResumeState{
		FilePath:            path,
		FileSize:            info.Size(),
		FileModTimeUnixNano: info.ModTime().UnixNano(),
		Checkpoints: []resumeCheckpoint{
			{LineNumber: 10, NextOffset: 123},
		},
	}

	plan := buildResumePlan(state, path, info, 5)
	if !plan.SkipUnchanged {
		t.Fatalf("SkipUnchanged = false, want true")
	}
}

func TestBuildResumePlanUsesReplayCheckpoint(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "meteo.dat")
	if err := os.WriteFile(path, []byte("header\nrow\nextra\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	state := &fileResumeState{
		FilePath:            path,
		FileSize:            info.Size() - 1,
		FileModTimeUnixNano: time.Now().Add(-time.Minute).UnixNano(),
		Checkpoints: []resumeCheckpoint{
			{LineNumber: 95, NextOffset: 950},
			{LineNumber: 96, NextOffset: 960},
			{LineNumber: 97, NextOffset: 970},
			{LineNumber: 98, NextOffset: 980},
			{LineNumber: 99, NextOffset: 990},
			{LineNumber: 100, NextOffset: 1000},
		},
	}

	plan := buildResumePlan(state, path, info, 3)
	if plan.StartLineNumber != 97 {
		t.Fatalf("StartLineNumber = %d, want 97", plan.StartLineNumber)
	}
	if plan.StartOffset != 970 {
		t.Fatalf("StartOffset = %d, want 970", plan.StartOffset)
	}
}

func TestTrimResumeCheckpointsKeepsReplayWindow(t *testing.T) {
	t.Parallel()

	checkpoints := []resumeCheckpoint{
		{LineNumber: 1, NextOffset: 10},
		{LineNumber: 2, NextOffset: 20},
		{LineNumber: 3, NextOffset: 30},
		{LineNumber: 4, NextOffset: 40},
	}

	got := trimResumeCheckpoints(checkpoints, 2)
	if len(got) != 3 {
		t.Fatalf("len(trimmed) = %d, want 3", len(got))
	}
	if got[0].LineNumber != 2 || got[2].LineNumber != 4 {
		t.Fatalf("trimmed line numbers = %#v", got)
	}
}
