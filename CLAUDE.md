# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Kapparmor is a Go application that dynamically loads/unloads AppArmor security profiles on Kubernetes cluster Linux nodes from a ConfigMap. It runs as a privileged DaemonSet, polling a mounted ConfigMap directory for profile changes and applying them via `apparmor_parser`.

**Language:** Go 1.25 | **Module:** `github.com/tuxerrante/kapparmor`

## Common Commands

```bash
# Run unit tests (verbose, with race detection)
make test

# Run tests with coverage
make test-coverage

# Run a single test
go test -v -run TestFunctionName ./src/app/...

# Format code
make fmt

# Lint (runs helm-lint, py-lint, go-lint)
make lint

# Go lint only
make go-lint

# Build Docker image
make docker-build

# Run all quality gates
make all

# E2E tests (requires MicroK8s cluster)
make e2e
make e2e-case1   # Profile management
make e2e-case2   # In-use profile deletion
make e2e-case3   # Prometheus metrics
```

## Architecture

The app is a single Go package (`package main`) in `src/app/`. Key flow:

1. **`main.go`** — Entry point. `RunApp()` starts the polling loop and handles graceful shutdown (SIGTERM/SIGINT). `pollProfiles()` runs on a ticker, calling `loadNewProfiles()` each cycle.
2. **`config.go`** — `AppConfig` struct initialized from env vars (`PROFILES_DIR`, `POLL_TIME`). Defaults: poll every 30s, profiles at `/app/profiles`, apparmor dir at `/etc/apparmor.d/custom`.
3. **`profiles_ops.go`** — Core profile operations: reads desired state from ConfigMap dir, reads current state from kernel (`/sys/kernel/security/apparmor/profiles`), diffs them via `calculateProfileChanges()`, and executes `apparmor_parser --replace` or `--remove`.
4. **`filesystemOperations.go`** — File operations: `CopyFile()`, `HasTheSameContent()`, profile validation (`isProfileNameCorrect`, `areProfilesReadable`).
5. **`metrics/metrics.go`** — Prometheus metrics for profile create/modify/delete operations.
6. **`healthz.go`** — HTTP health check server on port 8080.
7. **`const.go`** — Constants including `ProfileNamePrefix = "custom."` (all profiles must start with this).

### Key Design Decisions

- **No external dependencies beyond Prometheus client** — enforced by `depguard` linter rule in `.golangci.yml`. Only `$gostd` and `github.com/prometheus/client_golang` are allowed.
- **Profile naming convention** — all custom profiles must start with `custom.` prefix and filename must match profile name.
- **Thread safety** — `profileOperationsMutex` guards all profile load/unload operations.
- **Structured logging** — uses Go stdlib `log/slog`.

## Testing Conventions

- Test files use the `t_` prefix (e.g., `t_main_test.go`, `t_profiles_test.go`).
- Test helpers are in `t_helpers.go`.
- Profile test fixtures are in `src/app/profile_test_samples/`.
- Fuzz tests exist (e.g., `t_fuzzIsProfileNameCorrect_test.go`).
- The `TESTING=true` env var enables test-specific behavior (panic recovery, profile printing).
- Linters like `gosec` and `govet` are excluded for test files (configured in `.golangci.yml`).

## CI Pipeline (GitHub Actions)

Unit tests, fuzz tests, and coverage run **inside the Docker build** (Dockerfile build stage), not as a standalone workflow. The key CI workflows that gate PRs:

- **`build-app.yml`** (`1. Create app`) — Builds the Docker image targeting `test-coverage`, which executes `go test` (with coverage) and fuzz tests during the build. Uploads coverage to Codecov. Runs on pushes to `dev`, `feature/*`, and tags.
- **`integration-test.yml`** (`Integration test (AppArmor)`) — Builds the full Docker image (which also runs all unit/fuzz tests in the build stage), then runs the container with privileged host AppArmor mounts for smoke and integration testing. **Runs on PRs to `main`** — this is the primary quality gate that blocks merging if unit tests fail.
- **`golangci-lint.yml`** — Runs `golangci-lint` on PRs and pushes to `main`, `feature/*`, `fix/*`.
- **`ensure-sha-pinned-actions.yml`** — Validates all GitHub Actions use SHA-pinned references. Runs on PRs and pushes to `main` that touch `.github/workflows/`.
- **`codeql.yml`** — CodeQL static analysis for Go.
- **`scorecard.yml`** — OpenSSF Scorecard supply-chain security analysis.

**Important:** Do not create a separate unit test workflow — tests are already executed via the Dockerfile build stage in `build-app.yml` and `integration-test.yml`. Adding a standalone `go test` workflow would be redundant.

## Configuration

Shared build config lives in `config/config` (APP_VERSION, CHART_VERSION, GO_VERSION, POLL_TIME, HEALTHZPORT). The Makefile sources this file.

## Helm Chart

Located at `charts/kapparmor/`. Sample AppArmor profiles ship in `charts/kapparmor/profiles/`. Chart version must stay in sync with `config/config`.

## Development Principles

See `AGENTS.md` for full development philosophy, code quality standards, security requirements, and testing strategy. Key points relevant to daily work:

- Prefer Go standard libraries over external dependencies (enforced by `depguard`).
- Pre-commit hooks are configured (`.pre-commit-config.yaml`) — run `make precommit` to validate.
- Commits must be signed.
