VERSION := 0.4.0
COMMIT := $(shell git rev-parse HEAD)
BUILD_DATE := $(shell date +%F)
GO_BUILDOPTS := -ldflags "-s -w \
	-X github.com/falzm/fsdiff/internal/version.Version=$(VERSION) \
	-X github.com/falzm/fsdiff/internal/version.Commit=$(COMMIT) \
	-X github.com/falzm/fsdiff/internal/version.BuildDate=$(BUILD_DATE)"

.PHONY: help

build: ## Build the fsdiff command
	@echo "Building fsdiff command"
	@go build $(GO_BUILDOPTS) -o bin/fsdiff -mod=vendor

lint: ## Run linting checks
	@echo "Running linter"
	@golangci-lint run ./...

export PATH := $(PWD):$(PATH)
test: ## Run fsdiff command tests
	@echo "Running tests"
	@go test -v -race -count=1 ./...

clean: ## Clean compilation artefacts
	@rm -f bin/*

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: clean lint test
.DEFAULT_GOAL := help
