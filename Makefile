export

RED="\\033[31m"
GREEN="\\033[32m"
YELLOW="\\033[33m"
RESET="\\033[0m"

run: ### Running app
	go run cmd/main.go
.PHONY: run
