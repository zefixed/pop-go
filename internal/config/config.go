package config

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"log"
	"pop-go/internal/models"
	pkgfs "pop-go/pkg/fs"
	"slices"
	"strings"
)

func validateConfig() error {
	// Getting available locales
	langs, err := pkgfs.TakeSnapshot("./locales")
	if err != nil {
		return fmt.Errorf("error reading locales directory: %w", err)
	}

	// Checking that the specified locale is present in the locales
	if !slices.ContainsFunc(langs, func(s string) bool {
		return cfg.App.Lang == strings.TrimSuffix(s, ".yaml")
	}) {
		return fmt.Errorf("unknown language %q", cfg.App.Lang)
	}

	// Checking that all the directories specified in the config exist
	paths := []string{
		cfg.Obfuscator.TargetPath,
		cfg.Builder.OutputPath,
	}
	for _, path := range paths {
		exists, err := pkgfs.PathExists(cfg.App.TempDir)
		if err != nil {
			return err
		}

		if !exists {
			return fmt.Errorf("path does not exists: %s", path)
		}
	}

	// Checks for log {level, type}, obfuscator{literals{level}}, builder{goos, goarch}

	return nil
}

func GetConfig() (*models.Config, error) {
	return cfg, rootCmd.Execute()
}

var (
	configPath string
	cfg        = &models.Config{
		App: models.App{
			Name:    "pop-go",
			Version: "0.0.1",
		},
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
		Use:          "pop-go",
		Short:        "Go obfuscator",
		Long:         "Obfuscation program for applications written in the Go language",
		SilenceUsage: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return validateConfig()
		},
	}
)

func init() {
	initConfigPath()
	initApp()
	initLog()
	initObfuscator()
	initBuilder()

	err := viper.BindPFlags(rootCmd.Flags())
	if err != nil {
		log.Fatal("error bind flags")
	}

	err = viper.BindPFlags(rootCmd.PersistentFlags())
	if err != nil {
		log.Fatal("error bind persistent flags")
	}
}

func initConfig() error {
	path := viper.GetString("config")
	if path == "" {
		return nil
	}

	viper.SetConfigFile(path)
	viper.SetConfigType("json")

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("error reading config file %s: %w", path, err)
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("unable to decode config into struct: %w", err)
	}

	return nil
}

func initConfigPath() {
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "The path to the config file. If a path is specified, the config will be loaded from the file, and then the parameters passed by the flags will be overwritten.")
}

func initApp() {
	rootCmd.Flags().StringVarP(&cfg.App.Lang, "lang", "l", "en", "Logging language.")
	rootCmd.Flags().StringVar(&cfg.App.TempDir, "temp-dir", "", "Directory for storing temporary conversion files. By default, a unique directory will be generated in /tmp or C:\\Windows\\Temp and will be deleted if the --remove-temp=false flag is not set.")
	rootCmd.Flags().BoolVar(&cfg.App.RemoveTemp, "remove-temp", true, "Allows to avoid deleting the temp-dir after processing by setting it false.")
}

func initLog() {
	rootCmd.Flags().StringVar(&cfg.Log.Level, "log-level", "info", "Sets the logging level. Available levels: debug, info, warn, error.")
	rootCmd.Flags().StringVar(&cfg.Log.Type, "log-type", "text", "Sets the logging type. Available types: text, json.")
	rootCmd.Flags().BoolVar(&cfg.Log.EnableFile, "enable-log-file", false, "Enables logging to a file.")
	rootCmd.Flags().StringVar(&cfg.Log.File, "log-file", ".log", "A file for storing logs.")

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
	rootCmd.Flags().BoolVar(&cfg.Obfuscator.Comments.Enable, "delete-comments", false, "Removes comments from source code.")
}

func initLiterals() {
	rootCmd.Flags().BoolVar(&cfg.Obfuscator.Literals.Enable, "literals", false, "Obfuscates string literals.")
	rootCmd.Flags().StringVar(&cfg.Obfuscator.Literals.Level, "literals-level", "medium", "Literal obfuscation level (easy, medium).")
}

func initIdentifiers() {
	rootCmd.Flags().BoolVar(&cfg.Obfuscator.Identifiers.Enable, "identifiers", false, "Renames non-exported identifiers.")
}

func initControlFlow() {
	rootCmd.Flags().BoolVar(&cfg.Obfuscator.ControlFlow.Enable, "control-flow", false, "Enables control flow flattening obfuscation technique.")
}

func initBuilder() {
	rootCmd.Flags().StringSliceVar(&cfg.Builder.Flags, "builder-flags", []string{"-ldflags", "-s -w", "-trimpath"}, "Passes the flags to the go builder. Example: --builder-flags=\"-ldflags,-s -w,-trimpath\"")
	rootCmd.Flags().StringVar(&cfg.Builder.GOOS, "goos", "windows", "Target operating system (linux, windows).")
	rootCmd.Flags().StringVar(&cfg.Builder.GOARCH, "goarch", "amd64", "Target processor architecture (amd64, arm64).")
	rootCmd.Flags().StringVarP(&cfg.Builder.OutputPath, "output-path", "o", ".", "The path to save the compiled binary file.")
}
