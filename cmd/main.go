package main

import (
	"fmt"
	"pop-go/internal/app"
	"pop-go/internal/config"
)

func main() {
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	if err = config.ValidateConfig(cfg); err != nil {
		fmt.Println(err.Error())
		return
	}

	app.Run(cfg)
}
