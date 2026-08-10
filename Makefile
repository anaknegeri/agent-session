BINARY      := agent-session
MCP_BINARY  := agent-session-mcp
PLUGIN_DIR  := plugin
VERSION     ?= 0.1.0
INSTALL_DIR ?= $(HOME)/.local/bin
GOFLAGS     ?=

.PHONY: all build mcp install test vet lint fmt tidy plugin cross-compile clean

all: build mcp

build:
	go build -o bin/$(BINARY) ./cmd/agent-session

mcp:
	go build -o bin/$(MCP_BINARY) ./cmd/agent-session-mcp

install: build mcp
	mkdir -p $(INSTALL_DIR)
	cp bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	cp bin/$(MCP_BINARY) $(INSTALL_DIR)/$(MCP_BINARY)
	chmod +x $(INSTALL_DIR)/$(BINARY) $(INSTALL_DIR)/$(MCP_BINARY)
	@echo "installed to $(INSTALL_DIR)"

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd cli internal pkg

tidy:
	go mod tidy

plugin: build mcp
	@rm -rf $(PLUGIN_DIR)/bin
	@mkdir -p $(PLUGIN_DIR)/bin
	@cp bin/$(MCP_BINARY) $(PLUGIN_DIR)/bin/agent-session-mcp
	./bin/$(BINARY) plugin pack --binary bin/$(MCP_BINARY) --out $(PLUGIN_DIR) --version $(VERSION)

cross-compile:
	./scripts/cross-compile.sh

clean:
	rm -rf bin
