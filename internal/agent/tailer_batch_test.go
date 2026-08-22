package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadLinesHonorsBatchLimit(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "application.log")
	if writeError := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	lines, checkpoint, readError := ReadLines(path, Checkpoint{}, 1024, len("one\ntwo\n"), 100)
	if readError != nil {
		t.Fatal(readError)
	}
	if len(lines) != 2 || checkpoint.Offset != int64(len("one\ntwo\n")) {
		t.Fatalf("batch = %#v, checkpoint = %#v", lines, checkpoint)
	}
}
