# Developer entry points. Run `make help` for the list.

GO        ?= go
GOFLAGS   ?= -trimpath
BIN_DIR   := bin
GOLANGCI  ?= golangci-lint
GOVULN    ?= $(shell $(GO) env GOPATH)/bin/govulncheck

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Compile the server and the Inducks sync tool into bin/
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/server ./cmd/server
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/sync-inducks ./cmd/sync-inducks

.PHONY: web
web: ## Build the Preact client into dist/
	npm run build

.PHONY: run
run: build web ## Build everything and run the server on PORT (default 3000)
	./$(BIN_DIR)/server

.PHONY: dev
dev: ## Run the API and the Vite dev server together (restart after Go changes)
	npm run dev

.PHONY: sync
sync: build ## Download the Inducks dump and rebuild the local index
	./$(BIN_DIR)/sync-inducks

.PHONY: test
test: ## Run the Go test suite with the race detector
	$(GO) test -race -count=1 ./...

.PHONY: e2e
e2e: ## Run the browser tests (builds everything first)
	npm run test:e2e

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: lint
lint: ## Run golangci-lint (install: brew install golangci-lint)
	$(GOLANGCI) run ./...

.PHONY: vuln
vuln: ## Scan dependencies and code for known vulnerabilities
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest
	$(GOVULN) ./...

.PHONY: fmt-check
fmt-check: ## Fail if any Go file is not gofmt-formatted
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:" && gofmt -l . && exit 1)

.PHONY: check
check: fmt-check vet lint test vuln ## Everything CI runs for the Go code

.PHONY: clean
clean: ## Remove build output and test artefacts
	rm -rf $(BIN_DIR) dist tests/.tmp test-results playwright-report
