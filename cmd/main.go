package main

import (
	"pop-go/internal/app"
)

// TODO:
//   - Строковые литералы: сделать hard обфускацию
//   - Control Flow Flattening
//   - Вставка мертвого кода (сначала проверить не будет ли он удаляться компилятором)
func main() {
	app.Run()
}
