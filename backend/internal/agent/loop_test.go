package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type exporterProbe struct {
	batches []Batch
}

func (probe *exporterProbe) Export(_ context.Context, batch Batch) error {
	probe.batches = append(probe.batches, append(Batch(nil), batch...))
	return nil
}

func TestAgentLoopTickExportsAndCheckpoints(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "application.log")
	checkpointPath := filepath.Join(directory, "state", "checkpoint.json")
	if writeError := os.WriteFile(source, []byte("first\nsecond\n"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	probe := &exporterProbe{}
	loop, loopError := NewAgentLoop(source, checkpointPath, "", 0, 1024, 100, probe, NewMetrics())
	if loopError != nil {
		t.Fatal(loopError)
	}
	if tickError := loop.Tick(context.Background()); tickError != nil {
		t.Fatal(tickError)
	}
	if len(probe.batches) != 1 || len(probe.batches[0]) != 2 || probe.batches[0][0].Body != "first" {
		t.Fatalf("exported batches = %#v", probe.batches)
	}
	checkpoint, loadError := LoadCheckpoint(checkpointPath)
	if loadError != nil || checkpoint.Offset != int64(len("first\nsecond\n")) {
		t.Fatalf("checkpoint = %#v, error = %v", checkpoint, loadError)
	}
}

func TestAgentLoopDropsCorruptSpoolEntryAndContinues(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "application.log")
	if writeError := os.WriteFile(source, nil, 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	bufferDirectory := filepath.Join(directory, "buffer")
	if enqueueError := Enqueue(bufferDirectory, 1024, []byte("not-json")); enqueueError != nil {
		t.Fatal(enqueueError)
	}
	loop, loopError := NewAgentLoop(source, filepath.Join(directory, "checkpoint.json"), bufferDirectory, 1024, 1024, 100, &exporterProbe{}, NewMetrics())
	if loopError != nil {
		t.Fatal(loopError)
	}
	if tickError := loop.Tick(context.Background()); tickError != nil {
		t.Fatal(tickError)
	}
	payload, _, dequeueError := Dequeue(bufferDirectory)
	if dequeueError != nil || payload != nil {
		t.Fatalf("corrupt spool payload=%q error=%v", payload, dequeueError)
	}
}
