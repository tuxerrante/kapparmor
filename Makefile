SHELL := /bin/bash
# Source config/config variables
-include config/config
APP := kapparmor
PKG := ./src/app/...
BIN_DIR := ./.go/bin
COVER := coverage.out
GOLANGCI_LINT_VERSION ?= v2.6.0
GOLANGCI_LINT         := $(BIN_DIR)/golangci-lint

.PHONY: all fmt vet lint test test-coverage build docker-build docker-run docker-scan helm-lint precommit clean

all: fmt vet lint test-coverage docker-build docker-scan

fmt:
	@echo "> go fmt"
	gofmt -s -w src/ 
	@echo "> shfmt"
	shfmt --write --simplify -ln bash build/

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

docker-test:
	@echo "> docker build (test-coverage)"
	@docker build --target test-coverage --tag "ghcr.io/tuxerrante/$(APP):$(APP_VERSION)-dev" .

docker-build:
	@echo "> docker build - building production image"
	@docker build --tag "ghcr.io/tuxerrante/$(APP):$(APP_VERSION)-dev" \
		--build-arg POLL_TIME=$(POLL_TIME) \
		--build-arg PROFILES_DIR=/app/profiles \
		-f Dockerfile \
		.

docker-run: docker-build
	@echo "> docker run - testing container startup"
	@echo "> Detecting environment compatibility..."
	@if [ -d "/sys/kernel/security" ]; then \
		echo "> Starting container $(APP) with AppArmor mounts (native Linux)..."; \
		DOCKER_OPTS="--privileged --init --mount type=bind,source='/sys/kernel/security',target='/sys/kernel/security' --mount type=bind,source='/etc',target='/etc'"; \
	else \
		echo "> /sys/kernel/security not available (WSL2/Docker Desktop). Running in limited mode..."; \
		DOCKER_OPTS="--init"; \
	fi; \
	docker run --rm $$DOCKER_OPTS \
		--name "$(APP)-test" \
		--health-cmd='test -f /proc/self/cmdline' \
		--health-interval=5s \
		--health-timeout=3s \
		--health-retries=3 \
		"ghcr.io/tuxerrante/$(APP):$(APP_VERSION)-dev" &
	@sleep 10; \
	if docker inspect "$(APP)-test" > /dev/null 2>&1; then \
		echo "> Container is running. Checking logs..."; \
		docker logs "$(APP)-test" | head -30; \
		docker stop "$(APP)-test" || docker kill "$(APP)-test"; \
		echo "> ✓ Container started and ran successfully"; \
	else \
		echo "> ✗ Container failed to start"; \
		docker logs "$(APP)-test" 2>/dev/null || true; \
		exit 1; \
	fi

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
