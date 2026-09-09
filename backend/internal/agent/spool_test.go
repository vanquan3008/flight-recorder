package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBatchRoundTripPreservesRetryIdentity(t *testing.T) {
	batch := NewBatch([]string{"same", "same"})
	if batch[0].TimeUnixNano == 0 || batch[0].TimeUnixNano == batch[1].TimeUnixNano {
		t.Fatalf("batch timestamps are not unique: %#v", batch)
	}
	payload, encodeError := EncodeBatch(batch)
	if encodeError != nil {
		t.Fatal(encodeError)
	}
	decoded, decodeError := DecodeBatch(payload)
	if decodeError != nil {
		t.Fatal(decodeError)
	}
	if !reflect.DeepEqual(decoded, batch) {
		t.Fatalf("decoded batch = %#v, want %#v", decoded, batch)
	}
}

func TestDecodeBatchAcceptsLegacySpool(t *testing.T) {
	batch, decodeError := DecodeBatch([]byte(`["one","two"]`))
	if decodeError != nil || len(batch) != 2 || batch[0].Body != "one" || batch[1].Body != "two" {
		t.Fatalf("legacy batch = %#v, error=%v", batch, decodeError)
	}
}

func TestLoadCheckpointRejectsCorruptFile(t *testing.T) {
	path := t.TempDir() + "/checkpoint.json"
	if writeError := os.WriteFile(path, []byte("not-json"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	if _, loadError := LoadCheckpoint(path); loadError == nil {
		t.Fatal("expected corrupt checkpoint to fail")
	}
}

func TestEnqueueRejectsFullDiskBuffer(t *testing.T) {
	if enqueueError := Enqueue(t.TempDir(), 3, []byte("payload")); enqueueError == nil {
		t.Fatal("expected disk buffer limit to reject payload")
	}
}

func TestSpoolKeepsPayloadUntilAcknowledged(t *testing.T) {
	directory := t.TempDir()
	if enqueueError := Enqueue(directory, 1024, []byte("payload")); enqueueError != nil {
		t.Fatal(enqueueError)
	}
	payload, acknowledge, dequeueError := Dequeue(directory)
	if dequeueError != nil || !bytes.Equal(payload, []byte("payload")) {
		t.Fatalf("payload=%q error=%v", payload, dequeueError)
	}
	if acknowledgeError := acknowledge(); acknowledgeError != nil {
		t.Fatal(acknowledgeError)
	}
	payload, _, dequeueError = Dequeue(directory)
	if dequeueError != nil || payload != nil {
		t.Fatalf("payload=%q error=%v", payload, dequeueError)
	}
}

func TestDequeueDoesNotFollowSymlinks(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(t.TempDir(), "outside")
	if writeError := os.WriteFile(target, []byte("outside"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	link := filepath.Join(directory, "00000000-symlink")
	if symlinkError := os.Symlink(target, link); symlinkError != nil {
		t.Skipf("symlinks unavailable: %v", symlinkError)
	}
	payload, acknowledge, dequeueError := Dequeue(directory)
	if dequeueError != nil || payload != nil || acknowledge != nil {
		t.Fatalf("symlink was treated as a spool entry: payload=%q ack=%v error=%v", payload, acknowledge != nil, dequeueError)
	}
	if contents, readError := os.ReadFile(target); readError != nil || string(contents) != "outside" {
		t.Fatalf("symlink target was changed: %q error=%v", contents, readError)
	}
}
