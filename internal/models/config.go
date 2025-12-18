package models

type Config struct {
	App struct {
		Name         string `yaml:"name"`
		Version      string `yaml:"version"`
		Lang         string `yaml:"lang"`
		TempFolder   string `yaml:"temp_folder"`
		OutputFolder string `yaml:"output_folder"`
	} `yaml:"app"`

	Log struct {
		Level string `yaml:"level"`
		Type  string `yaml:"type"`
	} `yaml:"log"`

	Obfuscator struct {
		TargetPath string `yaml:"target_path"`
		Comments   bool   `yaml:"comments"`
		Seed       int64  `yaml:"seed"`
		Literals   struct {
			Level string `yaml:"level"`
		} `yaml:"literals"`
	} `yaml:"obfuscator"`

	Builder struct {
		Flags      []string `yaml:"flags"`
		GOOS       string   `yaml:"goos"`
		GOARCH     string   `yaml:"goarch"`
		OutputPath string   `yaml:"output_path"`
	} `yaml:"builder"`

	CurLocale map[string]string
}
