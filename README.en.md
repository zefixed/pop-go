# pop-go

**Русская версия:** [README.md](README.md)

A source-level obfuscator for Go projects.

## Table of Contents

- [Requirements](#requirements)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [CLI Flags](#cli-flags)
- [Config File](#config-file)
- [Transformations](#transformations)
- [How It Works](#how-it-works)
- [Limitations](#limitations)

## Requirements

- Go 1.22+
- The project must compile and pass `go test ./...` before running the obfuscator

## Installation

```bash
git clone https://github.com/zefixed/pop-go
cd pop-go
go build -o pop-go ./cmd
```

## Quick Start

Minimal run — identifier renaming, XOR string encryption, build for Linux amd64:

```bash
./pop-go \
  --target-path ./myproject \
  --output-path ./dist \
  --binary-name myapp \
  --identifiers \
  --literals --literals-level easy \
  --goos linux --goarch amd64
```

Full obfuscation via config file:

```bash
./pop-go -c config/config.maximum.json \
  --target-path ./myproject \
  --output-path ./dist \
  --binary-name myapp \
  --goos linux --goarch amd64
```

## CLI Flags

CLI flags always override values from the config file.

### General

| Flag | Default | Description |
|------|---------|-------------|
| `-c, --config` | — | Path to a JSON config file |
| `-t, --target-path` | — | Path to the project root (where `go.mod` lives) |
| `-o, --output-path` | `.` | Where to save the compiled binary |
| `--binary-name` | directory name | Name of the output binary |
| `-s, --seed` | `0` | PRNG seed. `0` = unique build, any other value = deterministic |
| `--test` | `true` | Run `go test ./...` before and after obfuscation |
| `--remove-temp` | `true` | Delete the temp directory when done |
| `--temp-dir` | — | Explicit path for the working temp directory |
| `-l, --lang` | `en` | Logger message language (`en`, `ru`) |

### Transformations

| Flag | Description |
|------|-------------|
| `--delete-comments` | Strip all comments from source files |
| `--identifiers` | Rename unexported identifiers |
| `--control-flow` | Apply control-flow flattening (CFF) |
| `--literals` | Encrypt string literals |
| `--literals-level` | Encryption profile: `easy` (XOR) or `medium` (AES-CTR) |

### Build

| Flag | Default | Description |
|------|---------|-------------|
| `--goos` | `windows` | Target operating system |
| `--goarch` | `amd64` | Target architecture |
| `--builder-flags` | `-ldflags -s -w -trimpath` | Flags passed to `go build` |

### Logging

| Flag | Default | Description |
|------|---------|-------------|
| `--log-level` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `--log-type` | `text` | Output format: `text` or `json` |
| `--enable-log-file` | `false` | Write logs to a file |
| `--log-file` | `.log` | Log file path |

## Config File

A JSON config file sets default values. CLI flags override them.

```json
{
  "app": {
    "lang": "en",
    "remove_temp": true,
    "test": true
  },
  "log": {
    "level": "info",
    "type": "text"
  },
  "obfuscator": {
    "target_path": "/path/to/project",
    "seed": 0,
    "comments":    { "enable": true  },
    "identifiers": { "enable": true  },
    "control_flow":{ "enable": true  },
    "literals":    { "enable": true, "level": "medium" }
  },
  "builder": {
    "flags": ["-ldflags", "-s -w", "-trimpath"],
    "goos": "linux",
    "goarch": "amd64",
    "output_path": "./dist",
    "binary_name": "myapp"
  }
}
```

Pre-made config files are available in the `config/` directory:

| File | Description                          |
|------|--------------------------------------|
| `config.base.json` | Comments + identifiers + XOR strings |
| `config.extended.json` | Base + AES strings                   |
| `config.maximum.json` | Extended + CFF              |

## Transformations

### Comment Deletion (`--delete-comments`)

Removes all doc-comments and inline comments from every source file.

### Identifier Renaming (`--identifiers`)

Replaces the names of all unexported symbols — functions, types, variables, struct fields, methods — with random strings of 16–32 characters.

Special cases handled correctly:
- Interface methods and their implementing struct methods receive the **same** new name so the interface is not broken.
- Embedded (anonymous) fields and their corresponding `TypeName` are kept in sync.
- Internal `_test.go` files (with `package foo`, not `package foo_test`) are renamed alongside the source files so that post-obfuscation tests still compile.

### Control-Flow Flattening (`--control-flow`)

Converts each function body into a `for`/`switch` dispatcher driven by a synthetic state variable. Every original statement becomes a separate `case`, making static control-flow analysis significantly harder.

Functions are skipped if they contain:
- goroutines or channel operations
- `goto` statements or labels
- local `const` or `type` declarations
- type assertions to anonymous structs

### String Literal Encryption (`--literals`)

Encrypts string literals at compile time and replaces them with calls to an inline decryption function injected into the same file.

| Level | Algorithm | Notes |
|-------|-----------|-------|
| `easy` | XOR | Single key byte, no extra imports |
| `medium` | AES-CTR | 32-byte key + 16-byte IV embedded in code |

Not encrypted: empty strings, struct field tags, import paths, compiler directives (`//go:`), type-conversion arguments.

## How It Works

1. Configuration is loaded and the logger is initialised
2. The project is copied to a temporary directory
3. If `--test`: `go test ./...` is run against the original source
4. Packages are loaded with full type information via `go/packages`
5. Transformations are applied in order: comments → identifiers → CFF → strings
6. Modified ASTs are written back to disk via `go/format`
7. If `--test`: `go test -vet=off ./...` is run against the obfuscated copy
8. `go build` is executed with the configured flags

## Limitations

- Only **unexported** identifiers are obfuscated. The exported API is never touched.
- Struct fields that share the same original name across different structs but were assigned different new names are not renamed in test files (safe fallback — the test will fail with a compile error only for those specific accesses).
- CFF does not process functions containing goroutines, channels, or several other constructs — they are left unchanged.