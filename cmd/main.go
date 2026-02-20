package main

import (
	"log"
	"pop-go/internal/app"
	"pop-go/internal/config"
)

func main() {
	cfg, err := config.NewConfig("config/config.json")
	if err != nil {
		log.Fatal(err.Error())
	}

	if err = config.ValidateConfig(cfg); err != nil {
		log.Fatal(err.Error())
	}

	app.Run(cfg)
}
