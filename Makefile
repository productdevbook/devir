VERSION ?= dev
BUILD_FLAGS = -ldflags "-s -w -X main.Version=$(VERSION)"

.PHONY: all build build-all clean test lint install

all: build

build:
	go build $(BUILD_FLAGS) -o devir ./cmd/devir

build-all: clean
	@mkdir -p dist
	GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/devir-linux-amd64 ./cmd/devir
	GOOS=linux GOARCH=arm64 go build $(BUILD_FLAGS) -o dist/devir-linux-arm64 ./cmd/devir
	GOOS=darwin GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/devir-darwin-amd64 ./cmd/devir
	GOOS=darwin GOARCH=arm64 go build $(BUILD_FLAGS) -o dist/devir-darwin-arm64 ./cmd/devir
	GOOS=windows GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/devir-windows-amd64.exe ./cmd/devir

clean:
	rm -rf dist/ devir

test:
	go test -v ./...

lint:
	golangci-lint run

install: build
	cp devir /usr/local/bin/devir

checksums:
	@cd dist && sha256sum * > checksums.txt && cat checksums.txt
