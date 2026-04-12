// Package fs provides filesystem utility functions used throughout pop-go.
package fs

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// CopyDir recursively copies the directory tree rooted at src into dst,
// preserving the relative path structure. Destination directories are created
// with permission 0755. Existing files in dst are overwritten.
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}
