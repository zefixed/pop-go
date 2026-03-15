package fs

import (
	"os"
	"path/filepath"
)

func TakeSnapshot(dir string) ([]string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	dirEntries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}

	var dirs []string
	for _, entry := range dirEntries {
		if !entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}

	return dirs, nil
}
