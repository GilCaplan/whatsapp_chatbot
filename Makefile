# WhatsApp Doppel — developer tasks. Run `make help` for a list.
# Works on macOS and Linux (Windows: scripts/dev.ps1). SQLite is pure Go
# (modernc.org/sqlite), so builds need no C compiler: CGO_ENABLED=0.

BIN        := build/whatsapp-doppel
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PROJECT    := $(abspath .)
LDFLAGS    := -X 'main.version=$(VERSION)' -X 'main.devProjectDir=$(PROJECT)'
DATA       ?= ./data
FAKE_DATA  ?= ./data/fake
UNAME      := $(shell uname -s)

.PHONY: help build run dev-fake test vet fmt app install uninstall icon smoke clean \
        dist dist-mac dist-linux dist-windows mac-only

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary into build/whatsapp-doppel
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .

run: build ## Run the real app with ./data and open the browser
	$(BIN) serve --data-dir $(DATA) --open

dev-fake: build ## Run with simulated WhatsApp + canned LLM (no phone, no Ollama)
	$(BIN) serve --fake-wa --fake-llm --data-dir $(FAKE_DATA) --open

test: ## Run all tests with the race detector (the race detector itself needs cgo)
	go test -race ./...

vet: ## go vet + gofmt check
	go vet ./...
	@out=$$(gofmt -l main.go internal scripts web); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

fmt: ## gofmt everything
	gofmt -w main.go internal scripts web

icon: ## Render build/icon_1024.png from scripts/geniconn
	go run ./scripts/geniconn -o build/icon_1024.png

mac-only:
	@if [ "$(UNAME)" != "Darwin" ]; then echo "This target builds the macOS .app and only works on a Mac. On Linux use: make run, make dist-linux or scripts/install-from-source.sh"; exit 1; fi

app: mac-only ## (Mac) Build build/WhatsappDoppel.app (icon, Info.plist, ad-hoc signature)
	VERSION=$(VERSION) scripts/build_app.sh

install: mac-only ## (Mac) Build and install to /Applications + Desktop alias + project symlink
	VERSION=$(VERSION) scripts/install_app.sh

uninstall: mac-only ## (Mac) Remove the installed app, alias and symlink (asks before deleting data)
	scripts/uninstall_app.sh

smoke: ## End-to-end test against a throwaway fake server
	scripts/smoke.sh

dist: ## Release archives for every OS into dist/ (the macOS .app part needs a Mac)
	VERSION=$(VERSION) scripts/dist.sh all

dist-mac: mac-only ## (Mac) dist/WhatsappDoppel-<ver>-macos-universal.zip
	VERSION=$(VERSION) scripts/dist.sh mac

dist-linux: ## dist/whatsapp-doppel-<ver>-linux-{amd64,arm64}.tar.gz
	VERSION=$(VERSION) scripts/dist.sh linux

dist-windows: ## dist/WhatsappDoppel-<ver>-windows-{amd64,arm64}.zip
	VERSION=$(VERSION) scripts/dist.sh windows

clean: ## Remove build output
	rm -rf build dist
