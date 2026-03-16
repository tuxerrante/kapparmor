# Contributing to Kapparmor

Thank you for your interest in contributing to Kapparmor! This document explains how to participate in the project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [How to Contribute](#how-to-contribute)
  - [Reporting Bugs](#reporting-bugs)
  - [Suggesting Features](#suggesting-features)
  - [Submitting Code Changes](#submitting-code-changes)
- [Development Setup](#development-setup)
- [Coding Standards](#coding-standards)
- [Testing Requirements](#testing-requirements)
- [Pull Request Process](#pull-request-process)
- [Security Vulnerabilities](#security-vulnerabilities)

---

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you agree to uphold it. Please report unacceptable behaviour to the project maintainers via a [GitHub issue](https://github.com/tuxerrante/kapparmor/issues).

---

## How to Contribute

### Reporting Bugs

1. **Search first** – Check [existing issues](https://github.com/tuxerrante/kapparmor/issues) to avoid duplicates.
2. **Open a new issue** with:
   - A clear, descriptive title
   - Steps to reproduce
   - Expected vs. actual behavior
   - Kapparmor version, Kubernetes version, and node OS
   - Relevant log output (sanitize sensitive data)

### Suggesting Features

Open a [GitHub Discussion](https://github.com/tuxerrante/kapparmor/discussions) or an issue labeled `enhancement`. Describe the use case and the value it brings.

### Submitting Code Changes

1. **Fork** the repository and create a feature branch from `main`:
   ```bash
   git checkout -b feature/my-feature
   ```
2. Make your changes following the [Coding Standards](#coding-standards).
3. Add or update tests as required (see [Testing Requirements](#testing-requirements)).
4. Ensure all quality gates pass locally:
   ```bash
   make fmt vet lint test-coverage
   ```
5. Commit with a signed commit (`git commit -s -S`) and a meaningful message.
6. Push your branch and open a Pull Request against `main`.
7. At least one maintainer review and approval is required before merging.

---

## Development Setup

### Prerequisites

- Go 1.25+
- Docker (for container builds)
- `make`
- `golangci-lint` (installed automatically by `make go-lint`)
- Helm 3 (for chart linting)
- A MicroK8s or Kubernetes cluster for end-to-end tests (optional)

### Quick Start

```bash
git clone https://github.com/tuxerrante/kapparmor.git
cd kapparmor

# Run unit tests
make test

# Run tests with coverage report
make test-coverage

# Lint all code
make lint

# Build Docker image locally
make docker-build
```

See [docs/testing.md](docs/testing.md) for detailed testing instructions including end-to-end tests.

---

## Coding Standards

- **Language**: Idiomatic Go following the [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).
- **Style**: `gofmt -s` must produce no output changes; run `make fmt` before committing.
- **Linting**: `make go-lint` must pass with zero errors.
- **Dependencies**: Prefer Go standard library. External runtime dependencies require explicit maintainer approval. The `depguard` linter enforces this.
- **Logging**: Use `log/slog` (Go stdlib structured logging). No `fmt.Print*` in production paths.
- **Error handling**: Always wrap errors with context; never silently discard errors.
- **Security**: Follow OWASP Go best practices. Run `gosec` locally if in doubt.
- **Documentation**: Public functions and types must have godoc comments.
- **Commits**: Must be signed (`git config commit.gpgsign true`).

---

## Testing Requirements

Every non-trivial change **must** include appropriate tests:

| Change type | Required tests |
|---|---|
| New feature | Unit tests + integration test if applicable |
| Bug fix | Regression test that would have caught the bug |
| Refactoring | Existing tests must continue to pass |
| Security fix | Test that demonstrates the vulnerability is fixed |

- Minimum coverage target: **80%** (enforced via Codecov).
- Test files use the `t_` name prefix (e.g., `t_myfeature_test.go`).
- Fuzz tests are encouraged for functions that process external input.
- Run `make test-coverage` to verify coverage locally.

---

## Pull Request Process

1. Keep PRs focused – one logical change per PR.
2. Ensure the CI pipeline passes (build, lint, test, security scan).
3. Update documentation (`README.md`, `docs/`) if your change affects user-facing behaviour.
4. Update `charts/kapparmor/CHANGELOG.md` for user-visible changes.
5. PRs require **at least one approving review** from a maintainer.
6. Squash-merge or rebase-merge is preferred to keep a clean history.

---

## Security Vulnerabilities

**Do not open a public issue for security vulnerabilities.**

Please follow the process described in [SECURITY.md](SECURITY.md) to report vulnerabilities privately.
