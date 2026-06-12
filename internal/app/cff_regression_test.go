package app_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunCFFHandlesLocalNewVariable(t *testing.T) {
	repoRoot := repositoryRoot(t)
	targetPath := filepath.Join(repoRoot, "internal", "app", "testdata", "cffshadow")

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir to repo root: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWD); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	}()

	t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "gocache"))

	buildFixture(t, targetPath, 1, buildOptions{
		controlFlow: true,
	})
}
