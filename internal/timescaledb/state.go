package timescaledb

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/atamostec/file-collector/internal/config"
)

type resumeCheckpoint struct {
	LineNumber int   `json:"line_number"`
	NextOffset int64 `json:"next_offset"`
}

type fileResumeState struct {
	InputName           string             `json:"input_name"`
	Schema              string             `json:"schema"`
	Table               string             `json:"table"`
	FilePath            string             `json:"file_path"`
	FileSize            int64              `json:"file_size"`
	FileModTimeUnixNano int64              `json:"file_mod_time_unix_nano"`
	UpdatedAt           time.Time          `json:"updated_at"`
	Checkpoints         []resumeCheckpoint `json:"checkpoints"`
}

type resumePlan struct {
	StartOffset     int64
	StartLineNumber int
	SkipUnchanged   bool
}

func defaultStateDirectory() string {
	if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
		return filepath.Join(homeDir, ".local", "state", "file-collector")
	}
	return filepath.Join(os.TempDir(), "file-collector-state")
}

func resolveStateDirectory(input config.Input) string {
	if input.StateDirectory != "" {
		return input.StateDirectory
	}
	return defaultStateDirectory()
}

func stateFilePath(input config.Input, output config.Output) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%s|%s|%s", input.Name, input.Directory, output.TimescaleDB.Schema, output.TimescaleDB.Table)))
	return filepath.Join(resolveStateDirectory(input), hex.EncodeToString(sum[:])+".json")
}

func loadResumeState(input config.Input, output config.Output) (*fileResumeState, error) {
	path := stateFilePath(input, output)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var state fileResumeState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func saveResumeState(input config.Input, output config.Output, state *fileResumeState) error {
	if state == nil {
		return nil
	}

	state.UpdatedAt = time.Now().UTC()
	path := stateFilePath(input, output)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func buildResumePlan(state *fileResumeState, filePath string, fileInfo os.FileInfo, replayLines int) resumePlan {
	plan := resumePlan{}
	if state == nil {
		return plan
	}
	if state.FilePath != filePath {
		return plan
	}
	if fileInfo.Size() < state.FileSize {
		return plan
	}
	if fileInfo.Size() == state.FileSize && fileInfo.ModTime().UnixNano() == state.FileModTimeUnixNano {
		plan.SkipUnchanged = true
		return plan
	}
	if len(state.Checkpoints) == 0 {
		return plan
	}

	checkpointIdx := len(state.Checkpoints) - 1 - replayLines
	if checkpointIdx < 0 {
		checkpointIdx = 0
	}
	checkpoint := state.Checkpoints[checkpointIdx]
	plan.StartOffset = checkpoint.NextOffset
	plan.StartLineNumber = checkpoint.LineNumber
	return plan
}

func trimResumeCheckpoints(checkpoints []resumeCheckpoint, replayLines int) []resumeCheckpoint {
	maxCheckpoints := replayLines + 1
	if maxCheckpoints < 1 {
		maxCheckpoints = 1
	}
	if len(checkpoints) <= maxCheckpoints {
		return checkpoints
	}
	return append([]resumeCheckpoint(nil), checkpoints[len(checkpoints)-maxCheckpoints:]...)
}
