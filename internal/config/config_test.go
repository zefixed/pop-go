package config

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"os"
	"pop-go/internal/models"
	"runtime"
	"strings"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name   string
		cfg    *models.Config
		errStr string
	}{
		{
			"Valid config",
			&models.Config{
				App: models.App{
					Name:    "pop-go",
					Version: "0.0.1",
					Lang:    "ru",
					TempDir: "",
				},
				Log: models.Log{
					Level: "debug",
					Type:  "text",
				},
				Obfuscator: models.Obfuscator{
					TargetPath: "../FQW",
					Comments:   models.Comments{Enable: true},
					Seed:       0,
					Literals: models.Literals{
						Level: "easy",
					},
				},
				Builder: models.Builder{
					Flags:      []string{"-ldflags", "-s -w", "-trimpath"},
					GOOS:       "windows",
					GOARCH:     "amd64",
					OutputPath: "/tmp",
				},
				CurLocale: nil,
			},
			"",
		},
		{
			"Invalid language",
			&models.Config{
				App: models.App{
					Name:    "pop-go",
					Version: "0.0.1",
					Lang:    "kz",
					TempDir: "",
				},
				Log: models.Log{
					Level: "debug",
					Type:  "text",
				},
				Obfuscator: models.Obfuscator{
					TargetPath: "../FQW",
					Comments:   models.Comments{Enable: true},
					Seed:       0,
					Literals: models.Literals{
						Level: "easy",
					},
				},
				Builder: models.Builder{
					Flags:      []string{"-ldflags", "-s -w", "-trimpath"},
					GOOS:       "windows",
					GOARCH:     "amd64",
					OutputPath: "/tmp",
				},
				CurLocale: nil,
			},
			"unknown language \"kz\"",
		},
		{
			"Empty TargetPath",
			&models.Config{
				App: models.App{
					Name:    "pop-go",
					Version: "0.0.1",
					Lang:    "ru",
					TempDir: "",
				},
				Log: models.Log{
					Level: "debug",
					Type:  "text",
				},
				Obfuscator: models.Obfuscator{
					TargetPath: "",
					Comments:   models.Comments{Enable: true},
					Seed:       0,
					Literals: models.Literals{
						Level: "easy",
					},
				},
				Builder: models.Builder{
					Flags:      []string{"-ldflags", "-s -w", "-trimpath"},
					GOOS:       "windows",
					GOARCH:     "amd64",
					OutputPath: "/tmp",
				},
				CurLocale: nil,
			},
			"target path is empty",
		},
		{
			"Invalid language and empty TargetPath",
			&models.Config{
				App: models.App{
					Name:    "pop-go",
					Version: "0.0.1",
					Lang:    "it",
					TempDir: "",
				},
				Log: models.Log{
					Level: "debug",
					Type:  "text",
				},
				Obfuscator: models.Obfuscator{
					TargetPath: "",
					Comments:   models.Comments{Enable: true},
					Seed:       0,
					Literals: models.Literals{
						Level: "easy",
					},
				},
				Builder: models.Builder{
					Flags:      []string{"-ldflags", "-s -w", "-trimpath"},
					GOOS:       "windows",
					GOARCH:     "amd64",
					OutputPath: "/tmp",
				},
				CurLocale: nil,
			},
			"unknown language \"it\" and target path is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getErr := ValidateConfig(tt.cfg)
			if tt.errStr != "" {
				if getErr != nil {
					if tt.errStr != getErr.Error() {
						t.Errorf("want %s, get %s", tt.errStr, getErr.Error())
					}
				} else {
					t.Errorf("want %s, get nothing", tt.errStr)
				}
			} else {
				if getErr != nil {
					t.Errorf("want nothing, get %s", getErr.Error())
				}
			}
		})
	}
}

func TestNewConfig(t *testing.T) {
	var tempDir string
	var err error

	// Making temp directory
	switch runtime.GOOS {
	case "linux":
		tempDir, err = os.MkdirTemp("/tmp", "")
	case "windows":
		tempDir, err = os.MkdirTemp("C:\\Windows\\Temp", "")
	default:
		err = fmt.Errorf("unknown os: %s", runtime.GOOS)
	}
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(tempDir)

	// Writing valid test config
	validTestConfigPath := "valid_test_config.json"
	err = os.WriteFile(tempDir+string(os.PathSeparator)+validTestConfigPath, []byte("{\n  \"app\": {\n    \"name\": \"pop-go\",\n    \"version\": \"0.0.1\",\n    \"lang\": \"ru\",\n    \"temp_folder\": \"\"\n  },\n  \"log\": {\n    \"level\": \"debug\",\n    \"type\": \"text\",\n    \"enable_log\": false,\n    \"file\": \".log\"\n  },\n  \"obfuscator\": {\n    \"target_path\": \"../FQW\",\n    \"seed\": 0,\n    \"comments\": {\n      \"enable\": false\n    },\n    \"literals\": {\n      \"level\": \"easy\"\n    }\n  },\n  \"builder\": {\n    \"flags\": [\"-ldflags\", \"-s -w\", \"-trimpath\"],\n    \"goos\": \"windows\",\n    \"goarch\": \"amd64\",\n    \"output_path\": \"/tmp\"\n  }\n}\n"), 0700)
	if err != nil {
		t.Error(err)
	}

	// Writing invalid test config
	invalidTestConfigPath := "invalid_test_config.json"
	err = os.WriteFile(tempDir+string(os.PathSeparator)+invalidTestConfigPath, []byte("{"), 0700)
	if err != nil {
		t.Error(err)
	}

	tests := []struct {
		name       string
		configPath string
		wantCfg    *models.Config
		errStr     string
	}{
		{
			"Empty config path",
			"",
			nil,
			"empty config path",
		},
		{
			"Invalid config",
			tempDir + string(os.PathSeparator) + invalidTestConfigPath,
			nil,
			"error unmarshal config",
		},
		{
			"Nonexistent file",
			tempDir + string(os.PathSeparator) + "nonexistent",
			nil,
			"error read config file: ",
		},
		{
			"Valid config",
			tempDir + string(os.PathSeparator) + validTestConfigPath,
			&models.Config{
				App: models.App{
					Name:    "pop-go",
					Version: "0.0.1",
					Lang:    "ru",
					TempDir: "",
				},
				Log: models.Log{
					Level:      "debug",
					Type:       "text",
					EnableFile: false,
					File:       ".log",
				},
				Obfuscator: models.Obfuscator{
					TargetPath: "../FQW",
					Comments:   models.Comments{Enable: false},
					Seed:       0,
					Literals: models.Literals{
						Enable: false,
						Level:  "easy",
					},
				},
				Builder: models.Builder{
					Flags:      []string{"-ldflags", "-s -w", "-trimpath"},
					GOOS:       "windows",
					GOARCH:     "amd64",
					OutputPath: "/tmp",
				},
				CurLocale: nil,
			},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newCfg, errCfg := NewConfig(tt.configPath)
			if tt.errStr != "" {
				if errCfg != nil {
					if !strings.Contains(errCfg.Error(), tt.errStr) {
						t.Errorf("want %s,\nget %s", tt.errStr, errCfg.Error())
					}
				} else {
					t.Errorf("want %s, get nothing", tt.errStr)
				}
			} else {
				if errCfg != nil {
					t.Errorf("want nothing, get %s", errCfg.Error())
				}
			}

			if tt.wantCfg != nil {
				if newCfg == nil {
					t.Errorf("want %v, get nothing", tt.wantCfg)
				}
				if !cmp.Equal(tt.wantCfg, newCfg) {
					t.Errorf("want %v,\nget %v", tt.wantCfg, newCfg)
				}
			}
		})
	}
}
