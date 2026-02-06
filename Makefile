BINARY    := flywork
MODULE    := github.com/fireflyframework/fireflyframework-cli
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE      := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS   := -s -w \
             -X '$(MODULE)/cmd.Version=$(VERSION)' \
             -X '$(MODULE)/cmd.GitCommit=$(COMMIT)' \
             -X '$(MODULE)/cmd.BuildDate=$(DATE)'

DIST      := dist
PLATFORMS := darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64 windows-arm64
INSTALL_DIR ?= /usr/local/bin

.PHONY: build install uninstall clean vet test build-all release checksums

# ── Local ────────────────────────────────────────────────────────────────────

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

install: build
	@echo "Installing $(BINARY) to $(INSTALL_DIR)/$(BINARY)"
	@mkdir -p $(INSTALL_DIR)
	cp bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "$(BINARY) installed successfully"
	@$(INSTALL_DIR)/$(BINARY) version

uninstall:
	@echo "Removing $(INSTALL_DIR)/$(BINARY)"
	rm -f $(INSTALL_DIR)/$(BINARY)
	@echo "$(BINARY) uninstalled"

clean:
	rm -rf bin/ $(DIST)/

vet:
	go vet ./...

test:
	go test ./...

# ── Cross-platform ──────────────────────────────────────────────────────────

build-all: clean
	@for platform in $(PLATFORMS); do \
		os=$${platform%%-*}; \
		arch=$${platform##*-}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" \
			-o $(DIST)/$(BINARY)-$(VERSION)-$$os-$$arch/$(BINARY)$$ext . || exit 1; \
	done

release: build-all
	@for platform in $(PLATFORMS); do \
		os=$${platform%%-*}; \
		arch=$${platform##*-}; \
		dir=$(DIST)/$(BINARY)-$(VERSION)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then \
			(cd $(DIST) && zip -qr $(BINARY)-$(VERSION)-$$os-$$arch.zip $(BINARY)-$(VERSION)-$$os-$$arch/); \
		else \
			tar -czf $(DIST)/$(BINARY)-$(VERSION)-$$os-$$arch.tar.gz -C $(DIST) $(BINARY)-$(VERSION)-$$os-$$arch; \
		fi; \
	done
	@echo "Release archives created in $(DIST)/"

checksums: release
	@cd $(DIST) && shasum -a 256 *.tar.gz *.zip > checksums.txt 2>/dev/null || true
	@echo "Checksums written to $(DIST)/checksums.txt"
