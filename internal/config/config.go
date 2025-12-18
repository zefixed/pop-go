package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"pop-go/internal/models"
	"strings"
)

func NewConfig() (*models.Config, error) {
	data, err := os.ReadFile("config/config.yaml")
	if err != nil {
		return nil, fmt.Errorf("error read config file: %w", err)
	}

	var cfg models.Config
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("error unmarshal config: %w", err)
	}

	return &cfg, nil
}

func ValidateConfig(cfg *models.Config) error {
	var errs []string

	langFlag := false
	for _, lang := range []string{"en", "ru"} {
		if cfg.App.Lang == lang {
			langFlag = true
		}
	}

	if !langFlag {
		errs = append(errs, fmt.Sprintf("unknown language %q", cfg.App.Lang))
	}

	if cfg.Obfuscator.TargetPath == "" {
		errs = append(errs, "target path is empty")
	}

	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, " and "))
	}

	return nil
}
