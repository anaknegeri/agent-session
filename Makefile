BINARY      := agent-session
MCP_BINARY  := agent-session-mcp
PLUGIN_DIR  := plugin
INSTALL_DIR ?= $(HOME)/.local/bin
GOFLAGS     ?=

# --- versioning -------------------------------------------------------------
# Single source of truth: scripts/version.sh (git tag) with ldflags injection.
VERSION_TAG ?= $(shell ./scripts/version.sh)
VERSION      := $(VERSION_TAG:v%=%)
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
BUILD_DATE  ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION_PKG  := github.com/anaknegeri/agent-session/pkg/version
LDFLAGS      := -s -w \
	-X $(VERSION_PKG).Version=$(VERSION) \
	-X $(VERSION_PKG).Commit=$(COMMIT) \
	-X $(VERSION_PKG).Date=$(BUILD_DATE)

.PHONY: all build mcp install test vet lint fmt tidy plugin cross-compile version demo clean

all: build mcp

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/agent-session

mcp:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(MCP_BINARY) ./cmd/agent-session-mcp

install: build mcp
	mkdir -p $(INSTALL_DIR)
	cp bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	cp bin/$(MCP_BINARY) $(INSTALL_DIR)/$(MCP_BINARY)
	chmod +x $(INSTALL_DIR)/$(BINARY) $(INSTALL_DIR)/$(MCP_BINARY)
	@echo "installed $(BINARY) $(VERSION) to $(INSTALL_DIR)"

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

version:
	@echo "tag:        $(VERSION_TAG)"
	@echo "version:    $(VERSION)"
	@echo "commit:     $(COMMIT)"
	@echo "build date: $(BUILD_DATE)"

# Re-renders the README demo. Needs vhs (brew install vhs) plus ttyd and ffmpeg;
# the tape builds its own binary and runs against a throwaway HOME.
demo:
	vhs docs/demo.tape

clean:
	rm -rf bin
