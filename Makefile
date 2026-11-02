GO_MODULES := admin client server shared
GO_COMMAND_MODULES := admin server
GO_LIBRARY_MODULES := client shared
RCC_INTEGRATION_SHARD ?= all

.PHONY: fmt test test-integration-runner test-integration test-browser-acceptance test-browser test-compose-migrations build lint lint-golangci lint-analyzers

# Pinned so local runs and CI fail on the same checks. Bump deliberately.
GOLANGCI_LINT_VERSION := v2.15.0
BIN_DIR := $(CURDIR)/bin
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint
RCCVET := $(BIN_DIR)/rccvet

fmt:
	@for module in $(GO_MODULES); do \
		(cd $$module && go fmt ./...); \
	done

test:
	@for module in $(GO_MODULES); do \
		(cd $$module && go test ./...) || exit $$?; \
	done

test-integration-runner:
	@python3 -m unittest scripts/tests/test_mysql_integration.py

test-integration:
	@docker info >/dev/null
	@if [ -n "$(RCC_INTEGRATION_ARTIFACTS)" ]; then \
		python3 scripts/mysql_integration.py --shard "$(RCC_INTEGRATION_SHARD)" --artifacts "$(RCC_INTEGRATION_ARTIFACTS)"; \
	else \
		python3 scripts/mysql_integration.py --shard "$(RCC_INTEGRATION_SHARD)"; \
	fi

test-browser-acceptance:
	@./scripts/browser-acceptance.sh

build:
	@for module in $(GO_LIBRARY_MODULES); do \
		(cd $$module && go build ./...) || exit $$?; \
	done
	@for module in $(GO_COMMAND_MODULES); do \
		mkdir -p bin/$$module || exit $$?; \
		(cd $$module && go build -o ../bin/$$module/$$module ./cmd/$$module) || exit $$?; \
	done
	@cd admin && go build -o ../bin/admin/account-maintain ./cmd/account-maintain
	@cd admin && go build -o ../bin/admin/schema-migrate ./cmd/schema-migrate
	@cd admin && go build -o ../bin/admin/policy-migrate ./cmd/policy-migrate
	@cd admin && go build -o ../bin/admin/release-reset ./cmd/release-reset

# Requires pnpm --dir web install and Chrome (or RCC_BROWSER_EXECUTABLE).
test-browser:
	@cd admin && go test -v -count=1 -timeout=45m -tags=integration,browser ./cmd/admin -run '^TestAccountBrowserSystemPath$$'

# Uses generated Compose project names and disposable volumes only.
test-compose-migrations:
	@python3 scripts/compose-migration-acceptance.py

# Hard gate: package boundaries, library print/panic, and any later bug-class rules.
lint: lint-golangci lint-analyzers

lint-golangci:
	@mkdir -p $(BIN_DIR)
	@if ! "$(GOLANGCI_LINT)" version 2>/dev/null | grep -q "version $(patsubst v%,%,$(GOLANGCI_LINT_VERSION)) "; then \
		GOTOOLCHAIN=go$$(awk '/^go /{print $$2; exit}' go.work) GOWORK=off GOBIN="$(BIN_DIR)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi
	"$(GOLANGCI_LINT)" run $(addprefix ./,$(addsuffix /...,$(GO_MODULES) tools/lint))

lint-analyzers:
	@mkdir -p $(BIN_DIR)
	go test -C tools/lint ./...
	go build -C tools/lint -o "$(RCCVET)" ./cmd/rccvet
	@for module in $(GO_MODULES); do \
		(cd $$module && go vet -vettool="$(RCCVET)" ./...) || exit $$?; \
	done
	go vet -C tools/lint -vettool="$(RCCVET)" ./...
