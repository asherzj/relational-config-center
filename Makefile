GO_MODULES := admin client server shared
GO_COMMAND_MODULES := admin server
GO_LIBRARY_MODULES := client shared

.PHONY: fmt test test-integration build

fmt:
	@for module in $(GO_MODULES); do \
		(cd $$module && go fmt ./...); \
	done

test:
	@for module in $(GO_MODULES); do \
		(cd $$module && go test ./...) || exit $$?; \
	done

test-integration:
	@cd admin && go test -count=1 -tags=integration ./...

build:
	@for module in $(GO_LIBRARY_MODULES); do \
		(cd $$module && go build ./...) || exit $$?; \
	done
	@for module in $(GO_COMMAND_MODULES); do \
		mkdir -p bin/$$module || exit $$?; \
		(cd $$module && go build -o ../bin/$$module/$$module ./cmd/$$module) || exit $$?; \
	done
