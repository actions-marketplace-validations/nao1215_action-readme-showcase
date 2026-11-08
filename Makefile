.PHONY: build test coverage e2e lint check clean help

APP     = readme-showcase
VERSION = $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)

build: ## Build the binary
	CGO_ENABLED=0 go build -trimpath -ldflags '-X main.version=$(VERSION)' -o $(APP) .

test: ## Run unit tests with coverage (writes cover.out / cover.html)
	go test -race -cover -coverprofile=cover.out ./...
	go tool cover -html=cover.out -o cover.html

coverage: ## Merge unit and E2E coverage into cover.out / cover.html (needs atago on PATH)
	bash scripts/coverage.sh

e2e: ## Run the atago end-to-end suite against a stub GitHub API (needs atago on PATH)
	atago run e2e/atago

lint: ## Run golangci-lint, shellcheck, and actionlint
	golangci-lint run ./...
	shellcheck -x scripts/*.sh
	actionlint

check: lint test e2e ## Run everything CI runs offline
	bash scripts/install_test.sh
	bash scripts/release_meta_test.sh

clean: ## Remove build and test output
	rm -rf $(APP) $(APP).exe dist third_party_licenses cover.out cover.html .atago .coverage

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-10s\033[0m %s\n", $$1, $$2}'
