package models

type Config struct {
	App        App        `json:"app" mapstructure:"app"`
	Log        Log        `json:"log" mapstructure:"log"`
	Obfuscator Obfuscator `json:"obfuscator" mapstructure:"obfuscator"`
	Builder    Builder    `json:"builder" mapstructure:"builder"`
	CurLocale  map[string]string
}

type App struct {
	Name       string `json:"name" mapstructure:"name"`
	Version    string `json:"version" mapstructure:"version"`
	Lang       string `json:"lang" mapstructure:"lang"`
	TempDir    string `json:"temp_dir" mapstructure:"temp_dir"`
	RemoveTemp bool   `json:"remove_temp" mapstructure:"remove_temp"`
	Test       bool   `json:"test" mapstructure:"test"`
}

type Log struct {
	Level      string `json:"level" mapstructure:"level"`
	Type       string `json:"type" mapstructure:"type"`
	EnableFile bool   `json:"enable_file" mapstructure:"enable_file"`
	File       string `json:"file" mapstructure:"file"`
}

type Obfuscator struct {
	// Ключевое исправление: добавлен mapstructure:"target_path"
	TargetPath  string      `json:"target_path" mapstructure:"target_path"`
	Seed        int64       `json:"seed" mapstructure:"seed"`
	Comments    Comments    `json:"comments" mapstructure:"comments"`
	Literals    Literals    `json:"literals" mapstructure:"literals"`
	Identifiers Identifiers `json:"identifiers" mapstructure:"identifiers"`
	ControlFlow ControlFlow `json:"control_flow" mapstructure:"control_flow"`
}

type Comments struct {
	Enable bool `json:"enable" mapstructure:"enable"`
}

type Literals struct {
	Enable bool   `json:"enable" mapstructure:"enable"`
	Level  string `json:"level" mapstructure:"level"`
}

type Identifiers struct {
	Enable bool `json:"enable" mapstructure:"enable"`
}

type ControlFlow struct {
	Enable bool `json:"enable" mapstructure:"enable"`
}

type Builder struct {
	Flags      []string `json:"flags" mapstructure:"flags"`
	GOOS       string   `json:"goos" mapstructure:"goos"`
	GOARCH     string   `json:"goarch" mapstructure:"goarch"`
	OutputPath string   `json:"output_path" mapstructure:"output_path"`
	BinaryName string   `json:"binary_name" mapstructure:"binary_name"`
}
