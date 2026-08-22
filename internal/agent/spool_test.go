package agent

import (
	"bytes"
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
