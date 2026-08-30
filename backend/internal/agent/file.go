package agent

import (
	"os"
	"path/filepath"
)

func writePrivateFile(path string, contents []byte) (returnError error) {
	directory := filepath.Dir(path)
	temporary, createError := os.CreateTemp(directory, "."+filepath.Base(path)+"-*")
	if createError != nil {
		return createError
	}
	temporaryPath := temporary.Name()
	defer func() {
		if returnError != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if chmodError := temporary.Chmod(0o600); chmodError != nil {
		_ = temporary.Close()
		return chmodError
	}
	if _, writeError := temporary.Write(contents); writeError != nil {
		_ = temporary.Close()
		return writeError
	}
	if syncError := temporary.Sync(); syncError != nil {
		_ = temporary.Close()
		return syncError
	}
	if closeError := temporary.Close(); closeError != nil {
		return closeError
	}
	return os.Rename(temporaryPath, path)
}
