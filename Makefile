VERSION := 0.2.0
COMMIT := $(shell git rev-parse HEAD)
BUILD_DATE := $(shell date +%F)
GO_BUILDOPTS := -ldflags "-s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildDate=$(BUILD_DATE)"

fsdiff:
	@go build $(GO_BUILDOPTS) -mod=vendor

export PATH := $(PWD):$(PATH)
test: fsdiff
	@./test.sh

clean:
	@rm -f fsdiff
