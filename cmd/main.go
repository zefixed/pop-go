// Package main is the entry point for the pop-go obfuscator CLI.
package main

import (
	"fmt"
	"os"
	"pop-go/internal/config"
)

// main initialises the CLI via [config.Execute] and exits with a non-zero
// status code if an error is returned.
func main() {
	if err := config.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
