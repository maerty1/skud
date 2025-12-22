# ND-Go Makefile for Windows

.PHONY: all build run clean test deps install fmt lint bin race linux windows static release help

# Build flags
LDFLAGS := -X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev") -X main.buildTime=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "unknown")
GCFLAGS :=

# Default target
all: deps build

# Create bin directory
bin:
	if not exist bin mkdir bin

# Download and tidy dependencies
deps:
	go mod download
	go mod tidy

# Build the project for Windows
build: bin
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -gcflags="$(GCFLAGS)" -o bin/nd-go.exe cmd/main.go

# Build static binary (no dependencies)
static: bin
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -a -ldflags "-extldflags '-static' $(LDFLAGS)" -gcflags="$(GCFLAGS)" -o bin/nd-go-static.exe cmd/main.go

# Build with race detector
race: bin
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build -race -ldflags "$(LDFLAGS)" -gcflags="$(GCFLAGS)" -o bin/nd-go-race.exe cmd/main.go

# Build for Linux
linux: bin
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -gcflags="$(GCFLAGS)" -o bin/nd-go-linux cmd/main.go

# Build for Windows (explicit target)
windows: build

# Run the project
run: build
	bin\nd-go.exe

# Install binary to system (requires admin rights)
install: build
	copy bin\nd-go.exe "C:\Program Files\nd-go\nd-go.exe"

# Clean build artifacts
clean:
	if exist bin rmdir /s /q bin
	go clean -cache -testcache -modcache

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Run vet
vet:
	go vet ./...

# Build release version (optimized)
release: bin
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w $(LDFLAGS)" -gcflags="$(GCFLAGS)" -o bin/nd-go.exe cmd/main.go

# Show help
help:
	@echo "Available targets:"
	@echo "  all          - Download deps and build (default)"
	@echo "  build        - Build for Windows amd64"
	@echo "  static       - Build static binary"
	@echo "  race         - Build with race detector"
	@echo "  linux        - Build for Linux amd64"
	@echo "  run          - Build and run the application"
	@echo "  install      - Install binary to Program Files (admin required)"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  test-coverage- Run tests with coverage report"
	@echo "  deps         - Download and tidy dependencies"
	@echo "  fmt          - Format code"
	@echo "  lint         - Run linter"
	@echo "  vet          - Run go vet"
	@echo "  release      - Build optimized release version"
	@echo "  help         - Show this help"
