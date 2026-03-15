package config

import (
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"path/filepath"
	"pop-go/internal/app"
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

	// Trimming path
	for i, lang := range langs {
		_, name := filepath.Split(lang)
		langs[i] = name
	}

	// Checking that the specified locale is present in the ./locales
	if !slices.ContainsFunc(langs, func(s string) bool {
		return cfg.App.Lang == strings.TrimSuffix(s, ".yaml")
	}) {
		return fmt.Errorf("unknown language %q", cfg.App.Lang)
	}

	// Checking that all the directories specified in the config exist
	paths := map[string]string{
		"target_path": cfg.Obfuscator.TargetPath,
		"output_path": cfg.Builder.OutputPath,
	}
	for name, path := range paths {
		if path == "" {
			return fmt.Errorf("%s is empty", name)
		}

		exists, err := pkgfs.PathExists(path)
		if err != nil {
			return err
		}

		if !exists {
			return fmt.Errorf("%s does not exists: %s", name, path)
		}
	}

	// Checking that log level is correct
	logLevels := []string{"debug", "info", "warn", "error"}
	if !slices.Contains(logLevels, cfg.Log.Level) {
		return fmt.Errorf(
			"unknown logging level %s, available (%s)",
			cfg.Log.Level,
			strings.Join(logLevels, ", "),
		)
	}

	// Checking that log type is correct
	logTypes := []string{"json", "text"}
	if !slices.Contains(logTypes, cfg.Log.Type) {
		return fmt.Errorf(
			"unknown logging type %s, available (%s)",
			cfg.Log.Type,
			strings.Join(logTypes, ", "),
		)
	}

	// Checking that obfuscation literals level is correct
	literalsLevel := []string{"easy", "medium"}
	if cfg.Obfuscator.Literals.Level != "" {
		if !slices.Contains(literalsLevel, cfg.Obfuscator.Literals.Level) {
			return fmt.Errorf(
				"unknown obfuscation literals level %s, available (%s)",
				cfg.Obfuscator.Literals.Level,
				strings.Join(literalsLevel, ", "),
			)
		}
	} else if cfg.Obfuscator.Literals.Enable {
		return errors.New("obfuscating literals is enabled but level is empty")
	}

	// Checking that GOOS is correct
	goos := []string{"aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "js", "linux", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows"}
	if !slices.Contains(goos, cfg.Builder.GOOS) {
		return fmt.Errorf(
			"unknown GOOS %s, available (%s)",
			cfg.Builder.GOOS,
			strings.Join(goos, ", "),
		)
	}

	// Checking that GOARCH is correct
	goarch := []string{"386", "amd64", "arm", "arm64", "loong64", "mips", "mips64", "mips64le", "mipsle", "ppc64", "ppc64le", "riscv64", "s390x", "wasm"}
	if !slices.Contains(goarch, cfg.Builder.GOARCH) {
		return fmt.Errorf(
			"unknown GOARCH %s, available (%s)",
			cfg.Builder.GOARCH,
			strings.Join(goarch, ", "),
		)
	}

	return nil
}

var (
	cfg = &models.Config{
		App: models.App{
			Name:    "pop-go",
			Version: "0.0.1",
		},
	}
	configPath string
	rootCmd    = &cobra.Command{
		Use:           "pop-go",
		Short:         "Go obfuscator",
		Long:          "Obfuscation program for applications written in the Go language",
		SilenceUsage:  true,
		SilenceErrors: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := initViperConfig(); err != nil {
				return err
			}
			if err := viper.Unmarshal(&cfg); err != nil {
				return fmt.Errorf("unable to decode config into struct: %w", err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateConfig(); err != nil {
				return err
			}
			return app.Run(cfg)
		},
	}
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	initConfigPath()
	initApp()
	initLog()
	initObfuscator()
	initBuilder()

	_ = viper.BindPFlags(rootCmd.Flags())
	_ = viper.BindPFlags(rootCmd.PersistentFlags())
}

func initViperConfig() error {
	if configPath == "" {
		return nil
	}
	viper.SetConfigFile(configPath)
	viper.SetConfigType("json")
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("error reading config file %s: %w", configPath, err)
	}
	return nil
}

func initConfigPath() {
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "The path to the config file. If a path is specified, the config will be loaded from the file, and then the parameters passed by the flags will be overwritten.")
	_ = viper.BindPFlag("config_path", rootCmd.PersistentFlags().Lookup("config"))
}

func initApp() {
	var lang, tempDir string
	var removeTemp bool
	rootCmd.Flags().StringVarP(&lang, "lang", "l", "en", "Logging language.")
	rootCmd.Flags().StringVar(&tempDir, "temp-dir", "", "Directory for storing temporary conversion files. By default, a unique directory will be generated in /tmp or C:\\Windows\\Temp and will be deleted if the --remove-temp=false flag is not set.")
	rootCmd.Flags().BoolVar(&removeTemp, "remove-temp", true, "Allows to avoid deleting the temp-dir after processing by setting it false.")
	_ = viper.BindPFlag("app.lang", rootCmd.Flags().Lookup("lang"))
	_ = viper.BindPFlag("app.temp_dir", rootCmd.Flags().Lookup("temp-dir"))
	_ = viper.BindPFlag("app.remove_temp", rootCmd.Flags().Lookup("remove-temp"))
}

func initLog() {
	var level, logType, file string
	var enableFile bool
	rootCmd.Flags().StringVar(&level, "log-level", "info", "Sets the logging level. Available levels: debug, info, warn, error.")
	rootCmd.Flags().StringVar(&logType, "log-type", "text", "Sets the logging type. Available types: text, json.")
	rootCmd.Flags().BoolVar(&enableFile, "enable-log-file", false, "Enables logging to a file.")
	rootCmd.Flags().StringVar(&file, "log-file", ".log", "A file for storing logs.")
	_ = viper.BindPFlag("log.level", rootCmd.Flags().Lookup("log-level"))
	_ = viper.BindPFlag("log.type", rootCmd.Flags().Lookup("log-type"))
	_ = viper.BindPFlag("log.enable_file", rootCmd.Flags().Lookup("enable-log-file"))
	_ = viper.BindPFlag("log.file", rootCmd.Flags().Lookup("log-file"))
}

func initObfuscator() {
	var targetPath string
	var seed int64
	rootCmd.Flags().StringVarP(&targetPath, "target-path", "t", "", "Path to the target root directory with the go.mod file.")
	rootCmd.Flags().Int64VarP(&seed, "seed", "s", 0, "Seed is used to randomize some processes. 0 to get unique builds, any other number to get deterministic builds.")
	_ = viper.BindPFlag("obfuscator.target_path", rootCmd.Flags().Lookup("target-path"))
	_ = viper.BindPFlag("obfuscator.seed", rootCmd.Flags().Lookup("seed"))
	initComments()
	initLiterals()
	initIdentifiers()
	initControlFlow()
}

func initComments() {
	var enable bool
	rootCmd.Flags().BoolVar(&enable, "delete-comments", false, "Removes comments from source code.")
	_ = viper.BindPFlag("obfuscator.comments.enable", rootCmd.Flags().Lookup("delete-comments"))
}

func initLiterals() {
	var enable bool
	var level string
	rootCmd.Flags().BoolVar(&enable, "literals", false, "Obfuscates string literals.")
	rootCmd.Flags().StringVar(&level, "literals-level", "medium", "Literal obfuscation level (easy, medium).")
	_ = viper.BindPFlag("obfuscator.literals.enable", rootCmd.Flags().Lookup("literals"))
	_ = viper.BindPFlag("obfuscator.literals.level", rootCmd.Flags().Lookup("literals-level"))
}

func initIdentifiers() {
	var enable bool
	rootCmd.Flags().BoolVar(&enable, "identifiers", false, "Renames non-exported identifiers.")
	_ = viper.BindPFlag("obfuscator.identifiers.enable", rootCmd.Flags().Lookup("identifiers"))
}

func initControlFlow() {
	var enable bool
	rootCmd.Flags().BoolVar(&enable, "control-flow", false, "Enables control flow flattening obfuscation technique.")
	_ = viper.BindPFlag("obfuscator.control_flow.enable", rootCmd.Flags().Lookup("control-flow"))
}

func initBuilder() {
	var flags []string
	var goos, goarch, outputPath string
	rootCmd.Flags().StringSliceVar(&flags, "builder-flags", []string{"-ldflags", "-s -w", "-trimpath"}, "Passes the flags to the go builder. Example: --builder-flags=\"-ldflags,-s -w,-trimpath\"")
	rootCmd.Flags().StringVar(&goos, "goos", "windows", "Target operating system (linux, windows).")
	rootCmd.Flags().StringVar(&goarch, "goarch", "amd64", "Target processor architecture (amd64, arm64).")
	rootCmd.Flags().StringVarP(&outputPath, "output-path", "o", ".", "The path to save the compiled binary file.")
	_ = viper.BindPFlag("builder.flags", rootCmd.Flags().Lookup("builder-flags"))
	_ = viper.BindPFlag("builder.goos", rootCmd.Flags().Lookup("goos"))
	_ = viper.BindPFlag("builder.goarch", rootCmd.Flags().Lookup("goarch"))
	_ = viper.BindPFlag("builder.output_path", rootCmd.Flags().Lookup("output-path"))
}
