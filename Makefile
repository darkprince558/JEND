# Makefile for JEND

# Variables
BINARY_NAME=jend
BUILD_DIR=bin
CMD_pkg=./cmd/jend
REGISTRY_PKG=./cmd/registry
TURN_AUTH_PKG=./cmd/turn-auth

# Build the main CLI
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_pkg)

# Build all binaries (including lambda functions)
.PHONY: build-all
build-all: build
	@echo "Building Registry Lambda..."
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/bootstrap $(REGISTRY_PKG)
	cd $(BUILD_DIR) && zip registry.zip bootstrap && rm bootstrap
	@echo "Building TURN Auth Lambda..."
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/turn-auth/bootstrap $(TURN_AUTH_PKG)
	cd $(BUILD_DIR)/turn-auth && zip ../turn-auth.zip bootstrap && rm bootstrap
	@rmdir $(BUILD_DIR)/turn-auth

# Run tests
.PHONY: test
test:
	go test ./... -v

# Run lint
.PHONY: lint
lint:
	golangci-lint run

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(BINARY_NAME)
	@rm -rf output
	@rm -rf cdk.out
	@rm -rf infra/cdk.out

# Install dependencies
.PHONY: deps
deps:
	go mod download

# Run the app
.PHONY: run
run: build
	./$(BUILD_DIR)/$(BINARY_NAME)
