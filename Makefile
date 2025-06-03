.PHONY: build run test clean install deps

BINARY_NAME=router
BIN_DIR=bin
INSTALL_PATH=/usr/local/bin

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY_NAME) cmd/router/main.go
	@echo "Build complete: $(BIN_DIR)/$(BINARY_NAME)"

run: build
	$(BIN_DIR)/$(BINARY_NAME)

install: build
	sudo cp $(BIN_DIR)/$(BINARY_NAME) $(INSTALL_PATH)/
	sudo chmod +x $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Installed to $(INSTALL_PATH)/$(BINARY_NAME)"

test:
	go test -v ./...

clean:
	go clean
	rm -rf $(BIN_DIR)

deps:
	go mod download
	go mod tidy

init-db: build
	$(BIN_DIR)/$(BINARY_NAME) -init-db

run-agi: build
	$(BIN_DIR)/$(BINARY_NAME) -agi -verbose

# Development helpers
dev-setup: deps build init-db
	@echo "Development environment ready!"

# Quick commands for development
provider-add: build
	@echo "Example: $(BIN_DIR)/$(BINARY_NAME) provider add s1 --type inbound --host 192.168.1.10"

did-add: build
	@echo "Example: $(BIN_DIR)/$(BINARY_NAME) did add 18001234567 --provider s3-1"

route-add: build
	@echo "Example: $(BIN_DIR)/$(BINARY_NAME) route add main s1 s3-1 s4-1"
