package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"pop-go/internal/models"
	"strings"
)

func NewConfig(path string) (*models.Config, error) {
	if path == "" {
		return nil, errors.New("empty config path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error read config file: %w", err)
	}

	var cfg models.Config
	if err = json.Unmarshal(data, &cfg); err != nil {
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
		return errors.New(strings.Join(errs, " and "))
	}

	return nil
}

func FillConfig() error {
	return rootCmd.Execute()
}

var (
	cfg = &models.Config{
		App: models.App{},
		Log: models.Log{},
		Obfuscator: models.Obfuscator{
			TargetPath:  "",
			Seed:        0,
			Comments:    models.Comments{},
			Literals:    models.Literals{},
			Identifiers: models.Identifiers{},
			ControlFlow: models.ControlFlow{},
		},
		Builder:   models.Builder{},
		CurLocale: nil,
	}
	rootCmd = &cobra.Command{
		Use:   "pop-go",
		Short: "Go obfuscator",
		Long:  "Obfuscation program for applications written in the Go language",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig(cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
)

func init() {
	initApp()
	initLog()
	initObfuscator()
	initBuilder()
}

func initApp() {
	rootCmd.Flags().StringVarP(&cfg.App.Lang, "lang", "l", "en", "Logging language.")
	rootCmd.Flags().StringVar(&cfg.App.TempDir, "temp-dir", "", "Directory for storing temporary conversion files. By default, a unique directory will be generated in /tmp or C:\\Windows\\Temp and will be deleted if the --remove-temp=false flag is not set.")
	rootCmd.Flags().BoolVar(&cfg.App.RemoveTemp, "remove-temp", true, "Allows to avoid deleting the temp-dir after processing by setting it false.")
}

func initLog() {
	rootCmd.Flags().StringVarP(&cfg.Log.Level, "log-level", "L", "info", "Sets the logging level. Available levels: debug, info, warn, error.")
	rootCmd.Flags().StringVarP(&cfg.Log.Type, "log-type", "T", "text", "Sets the logging type. Available types: text, json.")
	rootCmd.Flags().BoolVar(&cfg.Log.EnableFile, "enable-log-file", false, "Enables logging to a file.")
	rootCmd.Flags().StringVarP(&cfg.Log.File, "log-file", "f", ".log", "A file for storing logs.")

}

func initObfuscator() {
	rootCmd.Flags().StringVarP(&cfg.Obfuscator.TargetPath, "target-path", "t", "", "Path to the target root directory with the go.mod file.")
	rootCmd.Flags().Int64VarP(&cfg.Obfuscator.Seed, "seed", "s", 0, "Seed is used to randomize some processes. 0 to get unique builds, any other number to get deterministic builds.")
	initComments()
	initLiterals()
	initIdentifiers()
	initControlFlow()
}

func initComments() {

}

func initLiterals() {

}

func initIdentifiers() {

}
func initControlFlow() {

}

func initBuilder() {

}

func initConfig(cmd *cobra.Command) error {
	viper.BindPFlags(cmd.Flags())

	cfg.App.Lang = viper.GetString("lang")

	return nil
}
