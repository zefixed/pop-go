package main

import "fmt"

type worker struct {
	name  string
	count int
}

func (w *worker) message(prefix string) string {
	first := prefix + " " + w.name
	second := first + " #" + fmt.Sprint(w.count)
	third := second + " ready"
	return third
}

func main() {
	w := worker{name: "demo", count: 7}
	fmt.Println(w.message("hello"))
}
