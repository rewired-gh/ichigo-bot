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

build_all: pre build_linux build_darwin build_windows

build_linux: build_linux_amd64 build_linux_arm64

build_linux_amd64: pre
	GOOS=linux GOARCH=amd64 go build -o ./target/ichigod_linux_amd64 ./cmd/ichigod

build_linux_arm64: pre
	GOOS=linux GOARCH=arm64 go build -o ./target/ichigod_linux_arm64 ./cmd/ichigod

build_darwin: build_darwin_amd64 build_darwin_arm64

build_darwin_amd64: pre
	GOOS=darwin GOARCH=amd64 go build -o ./target/ichigod_darwin_amd64 ./cmd/ichigod

build_darwin_arm64: pre
	GOOS=darwin GOARCH=arm64 go build -o ./target/ichigod_darwin_arm64 ./cmd/ichigod

build_windows: build_windows_amd64

build_windows_amd64: pre
	GOOS=windows GOARCH=amd64 go build -o ./target/ichigod_windows_amd64.exe ./cmd/ichigod

.PHONY: pre dev build build_x64 build_all build_linux build_darwin build_windows