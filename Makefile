SHELL := /bin/bash
# Source config/config variables
-include config/config
APP := kapparmor
PKG := ./src/app/...
BIN_DIR := ./.go/bin
COVER := coverage.out
GOLANGCI_LINT_VERSION ?= v2.6.0
GOLANGCI_LINT         := $(BIN_DIR)/golangci-lint

.PHONY: all fmt vet lint test test-coverage build docker-build docker-scan helm-lint precommit clean

all: fmt vet lint test-coverage docker-build docker-scan

fmt:
	@echo "> go fmt"
	gofmt -s -w src/ 

vet:
	@echo "> go vet"
	@go vet ./src/...

lint: helm-lint
	@echo "> golangci-lint"
	@if [ ! -x $(BIN_DIR)/golangci-lint ]; then \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(BIN_DIR) $(GOLANGCI_LINT_VERSION); \
	fi
	@$(GOLANGCI_LINT) run --config .golangci.yml $(PKG)

test:
	@echo "> go test"
	@go test -v -race -timeout 2m $(PKG)

test-coverage:
	@echo "> go test with coverage"
	@go test -coverprofile=$(COVER) $(PKG)
	@go tool cover -func=$(COVER) | tail -n 1 || true

docker-build:
	@echo "> docker build (test-coverage)"
	@docker build --target test-coverage --tag "ghcr.io/tuxerrante/$(APP):$(APP_VERSION)-dev" .

docker-scan:
	@echo "> docker scout quickview"
	docker run --rm --name Grype anchore/grype:latest --fail-on high --platform linux/amd64 --sort-by severity "ghcr.io/${APP}:$(APP_VERSION)-dev"

helm-lint:
	@echo "> helm lint"
	@helm lint --values charts/kapparmor/values.yaml --kube-version 1.31 \
		--set=image.pullPolicy="Always",image.tag="0.3.0-dev",podAnnotations.gitCommit="a123" \
		charts/kapparmor/

precommit:
	@echo "> pre-commit run --all-files"
	@pre-commit run --all-files || true

clean:
	@rm -f $(COVER)
	@rm -rf ./.go/bin
