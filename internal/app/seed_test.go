package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"pop-go/internal/app"
	"pop-go/internal/models"
)

func TestRunRespectsSeedForBinaryDeterminism(t *testing.T) {
	repoRoot := repositoryRoot(t)
	targetPath := filepath.Join(repoRoot, "internal", "app", "testdata", "seedtarget")

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

	t.Run("with obfuscation passes", func(t *testing.T) {
		assertSeedBehavior(t, targetPath, buildOptions{
			identifiers: true,
			controlFlow: true,
			literals:    true,
		})
	})

	t.Run("without obfuscation passes", func(t *testing.T) {
		assertSeedBehavior(t, targetPath, buildOptions{})
	})
}

type buildOptions struct {
	identifiers bool
	controlFlow bool
	literals    bool
}

func assertSeedBehavior(t *testing.T, targetPath string, opts buildOptions) {
	t.Helper()

	firstSeeded := buildFixture(t, targetPath, 12345, opts)
	secondSeeded := buildFixture(t, targetPath, 12345, opts)
	if !bytes.Equal(firstSeeded, secondSeeded) {
		t.Fatal("same seed produced different binaries")
	}

	differentSeed := buildFixture(t, targetPath, 54321, opts)
	if bytes.Equal(firstSeeded, differentSeed) {
		t.Fatal("different seeds produced identical binaries")
	}

	firstUnique := buildFixture(t, targetPath, 0, opts)
	secondUnique := buildFixture(t, targetPath, 0, opts)
	if bytes.Equal(firstUnique, secondUnique) {
		t.Fatal("seed 0 should produce unique binaries")
	}
}

func buildFixture(t *testing.T, targetPath string, seed int64, opts buildOptions) []byte {
	t.Helper()

	outputDir := t.TempDir()
	binaryName := "seedtarget"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	cfg := &models.Config{
		App: models.App{
			Name:       "pop-go",
			Version:    "test",
			Lang:       "en",
			RemoveTemp: true,
			Test:       false,
		},
		Log: models.Log{
			Level: "error",
			Type:  "text",
		},
		Obfuscator: models.Obfuscator{
			TargetPath: targetPath,
			Seed:       seed,
			Identifiers: models.Identifiers{
				Enable: opts.identifiers,
			},
			ControlFlow: models.ControlFlow{
				Enable: opts.controlFlow,
			},
			Literals: models.Literals{
				Enable: opts.literals,
				Level:  "easy",
			},
		},
		Builder: models.Builder{
			Flags:      []string{"-ldflags", "-s -w", "-trimpath"},
			GOOS:       runtime.GOOS,
			GOARCH:     runtime.GOARCH,
			OutputPath: outputDir,
			BinaryName: binaryName,
		},
	}

	if err := app.Run(cfg); err != nil {
		t.Fatalf("run app with seed %d: %v", seed, err)
	}

	data, err := os.ReadFile(filepath.Join(outputDir, binaryName))
	if err != nil {
		t.Fatalf("read built binary for seed %d: %v", seed, err)
	}

	return data
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
