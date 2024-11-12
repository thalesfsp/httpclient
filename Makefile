###
# Params.
###

PROJECT_FULL_NAME := httpclient

HAS_GODOC := $(shell command -v godoc;)
HAS_GOLANGCI := $(shell command -v golangci-lint;)

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
ifndef HAS_GOLANGCI
	@echo "Could not find golangci-list, installing it"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0
endif
	@golangci-lint run -v -c .golangci.yml && echo "Lint OK"

test:
	@ENVIRONMENT="testing" configurer l d -f testing.env -- go test -timeout 60s -short -v -race -cover \
	-coverprofile=coverage.out ./... && echo "Test OK"

.PHONY: ci \
	coverage \
	doc \
	lint \
	test