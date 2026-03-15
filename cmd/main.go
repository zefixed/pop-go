package main

import (
	"fmt"
	"os"
	"pop-go/internal/config"
)

// TODO:
//   - Строковые литералы: сделать hard обфускацию
//   - Control Flow Flattening
//   - Вставка мертвого кода (сначала проверить не будет ли он удаляться компилятором)
func main() {
	if err := config.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
