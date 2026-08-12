GO_VERSION := 1.26.5
SQLC_VERSION := v1.31.1
GOIMPORTS_VERSION := v0.43.0
GOLANGCI_LINT_VERSION := v2.12.2
GOVULNCHECK_VERSION := v1.6.0

TOOLS_DIR := $(CURDIR)/.tools
SQLC := $(TOOLS_DIR)/sqlc-$(SQLC_VERSION)/sqlc
GOIMPORTS := $(TOOLS_DIR)/goimports-$(GOIMPORTS_VERSION)/goimports
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint-$(GOLANGCI_LINT_VERSION)/golangci-lint
GOVULNCHECK := $(TOOLS_DIR)/govulncheck-$(GOVULNCHECK_VERSION)/govulncheck
GO := env GOTOOLCHAIN=go$(GO_VERSION) go

UNIT_TEST_TIMEOUT ?= 2m
INTEGRATION_TEST_TIMEOUT ?= 10m
RACE_TEST_TIMEOUT ?= 10m

GO_FILES = $$(find . -type f -name '*.go' -not -path './tmp/*' -not -path './.tools/*' -not -path '*/gen/*')

FAST_GATES := format-check generate-check mod-check sqlc-vet lint build vuln-check test-unit test-integration test-race

.NOTPARALLEL:

.PHONY: tools fmt format-check generate generate-check tidy mod-check sqlc-vet lint build vuln-check test test-unit test-integration test-race docker-check verify verify-fast check run eval clean

tools: $(SQLC) $(GOIMPORTS) $(GOLANGCI_LINT) $(GOVULNCHECK)

$(SQLC):
	@mkdir -p $(@D)
	@GOBIN=$(@D) GOTOOLCHAIN=go$(GO_VERSION) go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)

$(GOIMPORTS):
	@mkdir -p $(@D)
	@GOBIN=$(@D) GOTOOLCHAIN=go$(GO_VERSION) go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)

$(GOLANGCI_LINT):
	@mkdir -p $(@D)
	@GOBIN=$(@D) GOTOOLCHAIN=go$(GO_VERSION) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOVULNCHECK):
	@mkdir -p $(@D)
	@GOBIN=$(@D) GOTOOLCHAIN=go$(GO_VERSION) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

fmt: $(GOIMPORTS)
	@$(GOIMPORTS) -local api -w $(GO_FILES)

format-check: $(GOIMPORTS)
	@files="$$($(GOIMPORTS) -local api -l $(GO_FILES))"; \
	if [ -n "$$files" ]; then \
		printf '%s\n' "Go files require formatting:" "$$files"; \
		exit 1; \
	fi

generate: $(SQLC)
	@$(SQLC) generate

generate-check: $(SQLC)
	@set -eu; \
	source_dir="$(CURDIR)"; \
	temp_dir="$$(mktemp -d)"; \
	trap 'rm -rf "$$temp_dir"' EXIT; \
	tar --exclude='./.tools' --exclude='./tmp' -cf - . | tar -xf - -C "$$temp_dir"; \
	find "$$temp_dir" -type d -name gen -prune -exec rm -rf '{}' '+'; \
	cd "$$temp_dir"; \
	$(SQLC) generate; \
	generated_dirs="$$( \
		{ \
			find "$$source_dir" -type d -name gen -not -path "$$source_dir/.tools/*" -print | sed "s#^$$source_dir/##"; \
			find "$$temp_dir" -type d -name gen -not -path "$$temp_dir/.tools/*" -print | sed "s#^$$temp_dir/##"; \
		} | sort -u \
	)"; \
	test -n "$$generated_dirs"; \
	printf '%s\n' "$$generated_dirs" | \
	while IFS= read -r relative_path; do \
		diff -ru "$$source_dir/$$relative_path" "$$temp_dir/$$relative_path" || exit 1; \
	done

tidy:
	@$(GO) mod tidy

mod-check:
	@$(GO) mod tidy -diff
	@$(GO) mod verify

sqlc-vet: $(SQLC)
	@$(SQLC) vet

lint: $(GOLANGCI_LINT)
	@$(GOLANGCI_LINT) run --timeout=5m

build:
	@$(GO) build ./...

vuln-check: $(GOVULNCHECK)
	@$(GOVULNCHECK) ./...

test: test-integration

test-unit:
	@$(GO) test -short -shuffle=on -count=1 -timeout=$(UNIT_TEST_TIMEOUT) ./...

test-integration:
	@$(GO) test -shuffle=on -count=1 -timeout=$(INTEGRATION_TEST_TIMEOUT) ./...

test-race:
	@$(GO) test -race -shuffle=on -count=1 -timeout=$(RACE_TEST_TIMEOUT) ./...

docker-check:
	@docker build --check --progress=plain .
	@set -eu; \
	image="linkedin-agent-api-verify:$$$$"; \
	trap 'docker image rm -f "$$image" >/dev/null 2>&1 || true' EXIT; \
	docker build --progress=plain --tag "$$image" .; \
	docker run --rm --entrypoint pdftotext "$$image" -v

verify-fast: $(FAST_GATES)
	@echo "all fast quality gates passed"

verify: $(FAST_GATES) docker-check
	@echo "all quality gates passed"

check: verify

run:
	@$(GO) run ./cmd/api

eval:
	@test -n "$$DATA_DIR" || { echo "eval: set DATA_DIR to a store this run may recreate"; exit 2; }
	@test -n "$$PORT" || { echo "eval: set PORT so the eval never shares the running service's port"; exit 2; }
	@$(GO) run ./cmd/eval

clean:
	@rm -rf "$(CURDIR)/tmp" "$(CURDIR)/.tools"
