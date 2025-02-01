.EXPORT_ALL_VARIABLES:

ICHIGOD_DATA_DIR := tmp

pre:
	go mod tidy
	mkdir -p ./target

dev:
	go run ./cmd/ichigod

debug:
	dlv debug ./cmd/ichigod

build: pre
	go build -o ./target ./cmd/ichigod

build_x64: pre
	GOOS=linux GOARCH=amd64 go build -o ./target/ichigod_linux_amd64 ./cmd/ichigod

.PHONY: pre dev build build_x64