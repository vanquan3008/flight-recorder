package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Record struct {
	TimeUnixNano uint64 `json:"time_unix_nano"`
	Body         string `json:"body"`
}

type Batch []Record

func NewBatch(lines []string) Batch {
	batch := make(Batch, len(lines))
	startedAt := uint64(time.Now().UnixNano())
	for index, line := range lines {
		batch[index] = Record{TimeUnixNano: startedAt + uint64(index), Body: line}
	}
	return batch
}

func EncodeBatch(batch Batch) ([]byte, error) { return json.Marshal(batch) }

func DecodeBatch(payload []byte) (Batch, error) {
	var batch Batch
	if decodeError := json.Unmarshal(payload, &batch); decodeError != nil {
		var legacyLines []string
		if legacyError := json.Unmarshal(payload, &legacyLines); legacyError != nil {
			return nil, decodeError
		}
		return NewBatch(legacyLines), nil
	}
	for _, record := range batch {
		if record.TimeUnixNano == 0 || record.TimeUnixNano > uint64(1<<63-1) {
			return nil, errors.New("agent spool record timestamp is invalid")
		}
	}
	return batch, nil
}

func Enqueue(directory string, maxBytes int64, payload []byte) error {
	if int64(len(payload)) > maxBytes {
		return errors.New("agent payload exceeds disk buffer limit")
	}
	if makeError := os.MkdirAll(directory, 0o700); makeError != nil {
		return makeError
	}
	entries, entriesError := os.ReadDir(directory)
	if entriesError != nil {
		return entriesError
	}
	var used int64
	for _, entry := range entries {
		path := filepath.Join(directory, entry.Name())
		info, infoError := os.Lstat(path)
		if infoError != nil {
			return infoError
		}
		if !info.Mode().IsRegular() {
			continue
		}
		used += info.Size()
	}
	if used+int64(len(payload)) > maxBytes {
		return errors.New("agent disk buffer is full")
	}
	var suffix [8]byte
	if _, randomError := rand.Read(suffix[:]); randomError != nil {
		return randomError
	}
	path := filepath.Join(directory, time.Now().UTC().Format("20060102T150405.000000000")+"-"+hex.EncodeToString(suffix[:]))
	return writePrivateFile(path, payload)
}

func Dequeue(directory string) ([]byte, func() error, error) {
	entries, readError := os.ReadDir(directory)
	if errors.Is(readError, os.ErrNotExist) {
		return nil, nil, nil
	}
	if readError != nil {
		return nil, nil, readError
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	if len(entries) == 0 {
		return nil, nil, nil
	}
	for _, entry := range entries {
		path := filepath.Join(directory, entry.Name())
		info, infoError := os.Lstat(path)
		if infoError != nil {
			return nil, nil, infoError
		}
		// Never follow symlinks or process directories placed in the spool. The
		// spool is an untrusted filesystem boundary and only regular files are
		// valid queue entries.
		if !info.Mode().IsRegular() {
			continue
		}
		payload, payloadError := os.ReadFile(path)
		if payloadError != nil {
			return nil, nil, payloadError
		}
		return payload, func() error { return os.Remove(path) }, nil
	}
	return nil, nil, nil
}
