package fs

import (
	"errors"
	"os"
	"path/filepath"
)

// PathExists reports whether the file or directory at path exists.
// The path is resolved to an absolute path before the check.
// Returns (false, nil) when the path is simply absent; returns (false, err)
// only for unexpected stat errors.
func PathExists(path string) (bool, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}

	_, err = os.Stat(abs)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
