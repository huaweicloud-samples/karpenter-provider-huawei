# Project variables
PROJECT_NAME := karpenter-provider-huawei
MODULE := github.com/huaweicloud/karpenter-provider-huawei
GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -X $(MODULE)/pkg/version.Version=$(VERSION) \
           -X $(MODULE)/pkg/version.GitCommit=$(GIT_COMMIT) \
           -X $(MODULE)/pkg/version.BuildDate=$(BUILD_DATE)

# Go variables
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: test
test: ## Run tests
	go test -race -coverprofile=coverage.out ./...

.PHONY: test-coverage
test-coverage: test ## Run tests and display coverage
	go tool cover -html=coverage.out -o coverage.html

##@ Build

.PHONY: build
build: fmt vet ## Build the controller binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o bin/controller ./cmd/controller

.PHONY: run
run: ## Run the controller locally
	go run ./cmd/controller

##@ Dependencies

.PHONY: tidy
tidy: ## Run go mod tidy
	go mod tidy

.PHONY: vendor
vendor: ## Run go mod vendor
	go mod vendor

.PHONY: verify
verify: tidy ## Verify dependencies
	git diff --exit-code go.mod go.sum

##@ Code Generation

.PHONY: generate
generate: ## Run code generators
	go generate ./...

.PHONY: manifests
manifests: ## Generate CRD manifests
	@echo "TODO: Add controller-gen for CRD generation"

##@ Cleanup

.PHONY: clean
clean: ## Clean build artifacts
	rm -rf bin/
	rm -rf _output/
	rm -f coverage.out coverage.html
