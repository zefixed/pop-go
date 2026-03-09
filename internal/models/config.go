package models

type Config struct {
	App        App        `json:"app"`
	Log        Log        `json:"log"`
	Obfuscator Obfuscator `json:"obfuscator"`
	Builder    Builder    `json:"builder"`
	CurLocale  map[string]string
}

type App struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Lang       string `json:"lang"`
	TempDir    string `json:"temp_dir"`
	RemoveTemp bool   `json:"remove_temp"`
}

type Log struct {
	Level      string `json:"level"`
	Type       string `json:"type"`
	EnableFile bool   `json:"enable_file"`
	File       string `json:"file"`
}

type Obfuscator struct {
	TargetPath  string      `json:"target_path"`
	Seed        int64       `json:"seed"`
	Comments    Comments    `json:"comments"`
	Literals    Literals    `json:"literals"`
	Identifiers Identifiers `json:"identifiers"`
	ControlFlow ControlFlow `json:"control_flow"`
}

type Comments struct {
	Enable bool `json:"enable"`
}

type Literals struct {
	Enable bool   `json:"enable"`
	Level  string `json:"level"`
}
type Identifiers struct {
	Enable bool `json:"enable"`
}

type ControlFlow struct {
	Enable bool `json:"enable"`
}

type Builder struct {
	Flags      []string `json:"flags"`
	GOOS       string   `json:"goos"`
	GOARCH     string   `json:"goarch"`
	OutputPath string   `json:"output_path"`
}
