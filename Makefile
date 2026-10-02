# WhatsApp Doppel — developer tasks. Run `make help` for a list.

BIN        := build/whatsapp-doppel
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PROJECT    := $(abspath .)
LDFLAGS    := -X 'main.version=$(VERSION)' -X 'main.devProjectDir=$(PROJECT)'
DATA       ?= ./data
FAKE_DATA  ?= ./data/fake
GO         := CGO_ENABLED=1 go

.PHONY: help build run dev-fake test vet fmt app install uninstall icon smoke clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary into build/whatsapp-doppel
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .

run: build ## Run the real app with ./data and open the browser
	$(BIN) serve --data-dir $(DATA) --open

dev-fake: build ## Run with simulated WhatsApp + canned LLM (no phone, no Ollama)
	$(BIN) serve --fake-wa --fake-llm --data-dir $(FAKE_DATA) --open

test: ## Run all tests with the race detector
	$(GO) test -race ./...

vet: ## go vet + gofmt check
	go vet ./...
	@out=$$(gofmt -l main.go internal scripts web); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

fmt: ## gofmt everything
	gofmt -w main.go internal scripts web

icon: ## Render build/icon_1024.png from scripts/geniconn
	go run ./scripts/geniconn -o build/icon_1024.png

app: ## Build build/WhatsappDoppel.app (icon, Info.plist, ad-hoc signature)
	VERSION=$(VERSION) scripts/build_app.sh

install: ## Build and install to /Applications + Desktop alias + project symlink
	VERSION=$(VERSION) scripts/install_app.sh

uninstall: ## Remove the installed app, alias and symlink (asks before deleting data)
	scripts/uninstall_app.sh

smoke: ## End-to-end test against a throwaway fake server
	scripts/smoke.sh

clean: ## Remove build output
	rm -rf build
