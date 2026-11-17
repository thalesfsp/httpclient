###
# Params.
###

PROJECT_FULL_NAME := httpclient

HAS_GODOC := $(shell command -v godoc;)

default: ci

###
# Entries.
###

ci: lint test coverage

coverage:
	@go tool cover -func=coverage.out && echo "Coverage OK"

doc:
ifndef HAS_GODOC
	@echo "Could not find godoc, installing it"
	@go install golang.org/x/tools/cmd/godoc@latest
endif
	@echo "Open localhost:6060/pkg/github.com/thalesfsp/$(PROJECT_FULL_NAME)/ in your browser\n"
	@godoc -http :6060

lint:
	@go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run \
	--allow-serial-runners --timeout 10m -v -c .golangci.yml && echo "Lint OK"

test:
	@ENVIRONMENT="testing" SYPL_LEVEL="debug" go test -timeout 60s -short -v -race -cover \
	-coverprofile=coverage.out ./... && echo "Test OK"

.PHONY: ci \
	coverage \
	doc \
	lint \
	test
