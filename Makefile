.PHONY: build build-windows build-all run run-dry test test-race test-verbose test-cover e2e e2e-install clean tidy lint help

# --- Variables ---
BINARY      := winctl
BIN_DIR     := bin
GO          := go
GOFLAGS     :=
PORT        ?= 8443
CONFIG      ?=

# Build flags
LDFLAGS     :=

# --- Build ---

build: tidy ## Build for current platform
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) .

build-windows: tidy ## Cross-compile for Windows amd64
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY).exe .

build-all: build build-windows ## Build for current platform and Windows

# --- Run ---

run: build ## Build and run in foreground mode
	$(BIN_DIR)/$(BINARY) run $(if $(CONFIG),-f $(CONFIG))

run-dry: build ## Build and run in dry-run mode
	$(BIN_DIR)/$(BINARY) run -d $(if $(CONFIG),-f $(CONFIG))

run-go: ## Run directly with go run (no build)
	$(GO) run . run $(if $(CONFIG),-f $(CONFIG))

run-go-dry: ## Run directly with go run in dry-run mode
	$(GO) run . run -d $(if $(CONFIG),-f $(CONFIG))

# --- Test ---

test: ## Run all Go tests
	$(GO) test ./... -count=1

test-race: ## Run all Go tests with race detector
	$(GO) test ./... -count=1 -race

test-verbose: ## Run all Go tests with verbose output
	$(GO) test ./... -count=1 -v

test-cover: ## Run tests with coverage report
	$(GO) test ./... -count=1 -coverprofile=$(BIN_DIR)/coverage.out
	$(GO) tool cover -func=$(BIN_DIR)/coverage.out
	@echo ""
	@echo "HTML report: go tool cover -html=$(BIN_DIR)/coverage.out"

# --- E2E (Playwright) ---

e2e-install: ## Install Playwright dependencies
	cd e2e && npm install && npx playwright install chromium

e2e: ## Run Playwright E2E tests (server must be running)
	cd e2e && WINCTL_PORT=$(PORT) npx playwright test

e2e-headed: ## Run Playwright E2E tests in headed mode
	cd e2e && WINCTL_PORT=$(PORT) npx playwright test --headed

e2e-debug: ## Run Playwright E2E tests in debug mode
	cd e2e && WINCTL_PORT=$(PORT) npx playwright test --debug

# --- Maintenance ---

tidy: ## Fetch and tidy Go module dependencies
	$(GO) mod tidy

lint: ## Run go vet
	$(GO) vet ./...

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
	rm -rf e2e/node_modules e2e/test-results e2e/playwright-report

# --- Help ---

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
