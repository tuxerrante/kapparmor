# Changelog

> All notable changes to this project will be documented in this file.  
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),  
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.1] - 2025-11

### Added

- **Prometheus Metrics Integration**: Complete metrics package enabling observability
- **E2E Test Suite Refactoring**: Production-ready Python replacement for shell tests
  - All original test cases preserved and enhanced (case1, case2, case3)
  - **NEW: Test Case 3 - Prometheus Metrics Validation**
    - Validates metrics endpoint (port 8080/metrics)
    - Verifies metrics collection: `kapparmor_profile_operations_total`, `kapparmor_profiles_managed`
    - Optional PromQL queries (gracefully skips if Prometheus not available)
    - Port-forward based testing (no external dependencies)
  - Added `--sideload` flag for offline testing (no GHCR token required)
  - Smart MicroK8s restart detection (compares normalized configs)
  - Two-phase Helm deployment with proper version validation
  - Dynamic imagePullPolicy selection (IfNotPresent for sideload, Always for GHCR)
  - Unified console+file logging with color support

## Previous Releases

- [0.3.0](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.3.0) - 2025-11-10
- [0.2.0](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.2.0) - 2024-02-19
- [0.1.5](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.5) - 2023-05-16
- [0.1.2](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.2) - 2023-02-16
- [0.1.1](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.1) - 2023-02-13
- [0.1.0](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.0) - 2023-02-01
- [0.0.6]() - 2023-01-26
- [0.0.5](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.0.5-alpha) - 2023-01-23

## TODO

- Generate signed OCI containers for all architectures
- Increase test coverage at least to 60%
- Implement [open telemetry](https://opentelemetry.io/docs/instrumentation/go/)
- Refactor code following [Google Go style guide](https://google.github.io/styleguide/go/guide)
