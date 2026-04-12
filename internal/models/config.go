// Package models defines the configuration data structures shared across all
// pop-go packages. Values are populated by the config package via Viper/Cobra
// and may also be loaded from a JSON config file.
package models

// Config is the root configuration object for a single obfuscation run.
type Config struct {
	App        App        `json:"app" mapstructure:"app"`
	Log        Log        `json:"log" mapstructure:"log"`
	Obfuscator Obfuscator `json:"obfuscator" mapstructure:"obfuscator"`
	Builder    Builder    `json:"builder" mapstructure:"builder"`
	CurLocale  map[string]string
}

// App holds general application settings: identity, language, and temp-dir
// lifecycle options.
type App struct {
	Name       string `json:"name" mapstructure:"name"`
	Version    string `json:"version" mapstructure:"version"`
	Lang       string `json:"lang" mapstructure:"lang"`
	TempDir    string `json:"temp_dir" mapstructure:"temp_dir"`
	RemoveTemp bool   `json:"remove_temp" mapstructure:"remove_temp"`
	Test       bool   `json:"test" mapstructure:"test"`
}

// Log configures the structured logger: verbosity level, output format, and
// optional file sink.
type Log struct {
	Level      string `json:"level" mapstructure:"level"`
	Type       string `json:"type" mapstructure:"type"`
	EnableFile bool   `json:"enable_file" mapstructure:"enable_file"`
	File       string `json:"file" mapstructure:"file"`
}

// Obfuscator groups all transformation options: the target project path,
// PRNG seed for deterministic output, and per-pass enable/configure flags.
type Obfuscator struct {
	TargetPath  string      `json:"target_path" mapstructure:"target_path"`
	Seed        int64       `json:"seed" mapstructure:"seed"`
	Comments    Comments    `json:"comments" mapstructure:"comments"`
	Literals    Literals    `json:"literals" mapstructure:"literals"`
	Identifiers Identifiers `json:"identifiers" mapstructure:"identifiers"`
	ControlFlow ControlFlow `json:"control_flow" mapstructure:"control_flow"`
}

// Comments controls whether source-code comments are stripped from the output.
type Comments struct {
	Enable bool `json:"enable" mapstructure:"enable"`
}

// Literals controls string-literal obfuscation and its encryption profile.
type Literals struct {
	Enable bool   `json:"enable" mapstructure:"enable"`
	Level  string `json:"level" mapstructure:"level"`
}

// Identifiers controls whether unexported identifiers are renamed.
type Identifiers struct {
	Enable bool `json:"enable" mapstructure:"enable"`
}

// ControlFlow controls whether control-flow flattening is applied.
type ControlFlow struct {
	Enable bool `json:"enable" mapstructure:"enable"`
}

// Builder holds the parameters forwarded to "go build": target OS/arch,
// extra flags, output path, and binary name.
type Builder struct {
	Flags      []string `json:"flags" mapstructure:"flags"`
	GOOS       string   `json:"goos" mapstructure:"goos"`
	GOARCH     string   `json:"goarch" mapstructure:"goarch"`
	OutputPath string   `json:"output_path" mapstructure:"output_path"`
	BinaryName string   `json:"binary_name" mapstructure:"binary_name"`
}
