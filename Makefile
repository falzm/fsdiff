VERSION := 0.3.0
COMMIT := $(shell git rev-parse HEAD)
BUILD_DATE := $(shell date +%F)
GO_BUILDOPTS := -ldflags "-s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildDate=$(BUILD_DATE) \
	-X github.com/falzm/fsdiff/snapshot.version=$(VERSION) \
	-X github.com/falzm/fsdiff/snapshot.commit=$(COMMIT)" \

.PHONY: help

fsdiff: ## Build the fsdiff command
	@echo "Building fsdiff command"
	@go build $(GO_BUILDOPTS) -mod=vendor

export PATH := $(PWD):$(PATH)
test: fsdiff ## Run fsdiff command tests
	@echo "Running tests"
	@./test.sh

clean: ## Clean compilation artefacts
	@rm -f fsdiff

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: clean test
.DEFAULT_GOAL := help
