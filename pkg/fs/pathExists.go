package fs

import (
	"errors"
	"os"
	"path/filepath"
)

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
