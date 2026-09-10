GO_MODULES := admin client server shared
GO_COMMAND_MODULES := admin server
GO_LIBRARY_MODULES := client shared

.PHONY: fmt test test-integration test-browser-acceptance test-browser test-compose-migrations build

fmt:
	@for module in $(GO_MODULES); do \
		(cd $$module && go fmt ./...); \
	done

test:
	@for module in $(GO_MODULES); do \
		(cd $$module && go test ./...) || exit $$?; \
	done

test-integration:
	@docker info >/dev/null
	@cd admin && go test -p 1 -count=1 -timeout=60m -tags=integration ./...

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

# Requires pnpm --dir web install and Chrome (or RCC_BROWSER_EXECUTABLE).
test-browser:
	@cd admin && go test -v -count=1 -timeout=5m -tags=integration,browser ./cmd/admin -run '^TestAccountBrowserSystemPath$$'

# Uses generated Compose project names and disposable volumes only.
test-compose-migrations:
	@python3 scripts/compose-migration-acceptance.py
