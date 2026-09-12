GO_MODULES := admin client server shared
GO_COMMAND_MODULES := admin server
GO_LIBRARY_MODULES := client shared
RCC_INTEGRATION_SHARD ?= all

.PHONY: fmt test test-integration-runner test-integration test-browser-acceptance test-browser test-compose-migrations build

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
