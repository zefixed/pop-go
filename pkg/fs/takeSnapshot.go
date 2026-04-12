package fs

import (
	"io/fs"
	"path/filepath"
)

// TakeSnapshot returns the absolute paths of all regular files inside dir,
// traversing subdirectories recursively. It is used to enumerate project
// files without loading them into memory.
func TakeSnapshot(dir string) ([]string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}
