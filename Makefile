.PHONY: build clean test run help

BINARY_NAME=proxy-ai
BIN_DIR=bin

# Default target
all: build

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/proxy-ai/main.go

## clean: Remove build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BIN_DIR)

## test: Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

## run: Build and run the binary in serve mode
run: build
	@echo "Running $(BINARY_NAME) serve..."
	@./$(BIN_DIR)/$(BINARY_NAME) serve

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' Makefile | column -t -s ':' |  sed -e 's/^/ /'
