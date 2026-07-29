GOLANGCI_VERSION ?= v2.12.2

# The sibling JavaScript implementation. Set it to enable the cross-language
# parity checks; they skip cleanly when it is unset or absent.
JS_VALIDATOR_REPO ?= $(HOME)/code/aigentflow-flow-validator-js

.PHONY: help
help: ## Show this help.
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: test
test: ## Run the test suite with the race detector.
	go test -race -count=1 ./...

.PHONY: lint
lint: ## Run golangci-lint.
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Format all Go files.
	gofmt -w .

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: wasm-check
wasm-check: ## Verify the library compiles for GOOS=js GOARCH=wasm.
	@# The consumers include a browser WASM terminal, so a stray os/net import
	@# must fail here rather than at the consumer's build.
	GOOS=js GOARCH=wasm go build ./...

.PHONY: parity-check
parity-check: ## Diff this implementation against the sibling JS one (needs JS_VALIDATOR_REPO).
	@# Requires a FRESH JS build: the harness reads the built CLI's own tracked
	@# spec version and skips (loudly) when it trails this library's, because a
	@# stale dist/ would otherwise report as parity drift.
	@if [ ! -d "$(JS_VALIDATOR_REPO)" ]; then \
	  echo "parity-check: $(JS_VALIDATOR_REPO) not found; set JS_VALIDATOR_REPO=/path/to/aigentflow-flow-validator-js"; \
	  exit 1; \
	fi
	@echo "parity-check: building the JS validator so the comparison is against current source"
	cd "$(JS_VALIDATOR_REPO)" && npm run build
	AIF_JS_VALIDATOR_REPO="$(JS_VALIDATOR_REPO)" go test -count=1 -v -run Parity ./...

.PHONY: tools
tools: ## Install the pinned golangci-lint.
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)

.PHONY: ci
ci: fmt vet lint test wasm-check ## The full gate. Pass before pushing.
	@echo "ci: green"
