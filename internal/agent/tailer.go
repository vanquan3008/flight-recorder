package agent

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

type Checkpoint struct {
	Offset       int64  `json:"offset"`
	PrefixHash   string `json:"prefix_hash"`
	PrefixLength int64  `json:"prefix_length"`
}

func LoadCheckpoint(path string) (Checkpoint, error) {
	contents, readError := os.ReadFile(path)
	if errors.Is(readError, os.ErrNotExist) {
		return Checkpoint{}, nil
	}
	if readError != nil {
		return Checkpoint{}, readError
	}
	var checkpoint Checkpoint
	if decodeError := json.Unmarshal(contents, &checkpoint); decodeError != nil || checkpoint.Offset < 0 {
		return Checkpoint{}, errors.New("invalid agent checkpoint")
	}
	return checkpoint, nil
}

func SaveCheckpoint(path string, checkpoint Checkpoint) error {
	contents, marshalError := json.Marshal(checkpoint)
	if marshalError != nil {
		return marshalError
	}
	if makeError := os.MkdirAll(filepath.Dir(path), 0o700); makeError != nil {
		return makeError
	}
	return writePrivateFile(path, contents)
}

func ReadLines(path string, checkpoint Checkpoint, maxLineBytes, maxBatchBytes, maxBatchLines int) ([]string, Checkpoint, error) {
	if maxLineBytes <= 0 || maxBatchBytes <= 0 || maxBatchLines <= 0 {
		return nil, checkpoint, errors.New("agent read limits must be positive")
	}
	info, statError := os.Stat(path)
	if statError != nil {
		return nil, checkpoint, statError
	}
	if info.Size() < checkpoint.Offset {
		checkpoint.Offset = 0
	}
	file, openError := os.Open(path)
	if openError != nil {
		return nil, checkpoint, openError
	}
	defer file.Close()
	if checkpoint.PrefixLength > 0 {
		hash, hashError := prefixHash(file, checkpoint.PrefixLength)
		if hashError != nil {
			return nil, checkpoint, hashError
		}
		if hash != checkpoint.PrefixHash {
			checkpoint.Offset, checkpoint.PrefixLength, checkpoint.PrefixHash = 0, 0, ""
		}
	}
	if _, seekError := file.Seek(checkpoint.Offset, io.SeekStart); seekError != nil {
		return nil, checkpoint, seekError
	}
	reader := bufio.NewReader(file)
	lines := make([]string, 0)
	batchBytes := 0
	for {
		line, readError := readLine(reader, maxLineBytes)
		if len(line) > 0 && line[len(line)-1] == '\n' && (len(lines) == maxBatchLines || batchBytes+len(line) > maxBatchBytes) {
			if len(lines) == 0 {
				return nil, checkpoint, errors.New("log line exceeds configured batch limit")
			}
			return lines, withPrefix(file, checkpoint), nil
		}
		if len(line) > 0 && line[len(line)-1] == '\n' {
			lines = append(lines, line[:len(line)-1])
			checkpoint.Offset += int64(len(line))
			batchBytes += len(line)
		}
		if errors.Is(readError, io.EOF) {
			return lines, withPrefix(file, checkpoint), nil
		}
		if readError != nil {
			return nil, checkpoint, readError
		}
	}
}

func readLine(reader *bufio.Reader, maxLineBytes int) (string, error) {
	line := make([]byte, 0)
	for {
		fragment, readError := reader.ReadSlice('\n')
		line = append(line, fragment...)
		if len(line) > maxLineBytes {
			return "", errors.New("log line exceeds configured limit")
		}
		if !errors.Is(readError, bufio.ErrBufferFull) {
			return string(line), readError
		}
	}
}

func withPrefix(file *os.File, checkpoint Checkpoint) Checkpoint {
	length := checkpoint.Offset
	if length > 4096 {
		length = 4096
	}
	if length > 0 {
		checkpoint.PrefixLength = length
		checkpoint.PrefixHash, _ = prefixHash(file, length)
	}
	return checkpoint
}

func prefixHash(file *os.File, length int64) (string, error) {
	if _, seekError := file.Seek(0, io.SeekStart); seekError != nil {
		return "", seekError
	}
	contents, readError := io.ReadAll(io.LimitReader(file, length))
	if readError != nil {
		return "", readError
	}
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:]), nil
}
