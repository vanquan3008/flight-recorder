package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadLinesResumesAndHandlesTruncate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if writeError := os.WriteFile(path, []byte("one\ntwo\npartial"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	lines, checkpoint, _, readError := ReadLines(path, Checkpoint{}, 1024, 4096, 100)
	if readError != nil || len(lines) != 2 {
		t.Fatalf("lines=%v error=%v", lines, readError)
	}
	checkpointPath := filepath.Join(filepath.Dir(path), "checkpoint.json")
	if saveError := SaveCheckpoint(checkpointPath, checkpoint); saveError != nil {
		t.Fatal(saveError)
	}
	checkpoint, readError = LoadCheckpoint(checkpointPath)
	if readError != nil {
		t.Fatal(readError)
	}
	if writeError := os.WriteFile(path, []byte("new\n"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	lines, _, _, readError = ReadLines(path, checkpoint, 1024, 4096, 100)
	if readError != nil || len(lines) != 1 || lines[0] != "new" {
		t.Fatalf("lines=%v error=%v", lines, readError)
	}
}

func TestReadLinesDetectsReplacementLargerThanCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if writeError := os.WriteFile(path, []byte("old\n"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	_, checkpoint, _, readError := ReadLines(path, Checkpoint{}, 1024, 4096, 100)
	if readError != nil {
		t.Fatal(readError)
	}
	if writeError := os.WriteFile(path, []byte("replacement-is-longer\n"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	lines, _, _, readError := ReadLines(path, checkpoint, 1024, 4096, 100)
	if readError != nil || len(lines) != 1 || lines[0] != "replacement-is-longer" {
		t.Fatalf("lines=%v error=%v", lines, readError)
	}
}

func TestReadLinesSkipsOversizedLineAndAdvancesCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	contents := []byte("this-line-is-too-long\nvalid\n")
	if writeError := os.WriteFile(path, contents, 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	lines, checkpoint, skipped, readError := ReadLines(path, Checkpoint{}, 8, 4096, 100)
	if readError != nil || skipped != 1 || len(lines) != 1 || lines[0] != "valid" {
		t.Fatalf("lines=%v checkpoint=%#v skipped=%d error=%v", lines, checkpoint, skipped, readError)
	}
	if checkpoint.Offset != int64(len(contents)) {
		t.Fatalf("checkpoint offset=%d want=%d", checkpoint.Offset, len(contents))
	}
}

func TestSaveCheckpointCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "checkpoint.json")
	want := Checkpoint{Offset: 42}
	if saveError := SaveCheckpoint(path, want); saveError != nil {
		t.Fatal(saveError)
	}
	got, loadError := LoadCheckpoint(path)
	if loadError != nil || got.Offset != want.Offset {
		t.Fatalf("checkpoint = %#v, error=%v", got, loadError)
	}
}
