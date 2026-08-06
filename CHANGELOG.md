# Changelog

All notable changes to this project will be documented in this file.  
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),  
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **Helm chart changelog:** See [`charts/kapparmor/CHANGELOG.md`](charts/kapparmor/CHANGELOG.md) for Helm-chart-specific changes.

---

## [Unreleased]

### Added
- `CONTRIBUTING.md` – contribution guidelines
- `CODE_OF_CONDUCT.md` – Contributor Covenant v2.1
- `CHANGELOG.md` – root-level project changelog
- `SECURITY.md` – private vulnerability reporting via GitHub Security Advisories

---

## [0.3.1] - 2025-11-22

### Added
- Prometheus metrics integration (`kapparmor_profile_operations_total`, `kapparmor_profiles_managed`)
- End-to-end test suite rewrite in Python (cases 1–3, with Prometheus metrics validation)
- `--sideload` flag for offline e2e testing
- `healthz` HTTP endpoint on port 8080

### Changed
- Improved MicroK8s restart detection in e2e tests
- Two-phase Helm deployment with proper version validation

---

## [0.3.0] - 2025-11-10

See [GitHub release](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.3.0).

---

## [0.2.1] - 2025-11-07

See [GitHub release](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.2.1).

---

## [0.1.9] - 2024-02-19

See [GitHub release](https://github.com/tuxerrante/kapparmor/releases/tag/v0.1.9).

---

## [0.1.7] - 2025-05-07

See [GitHub release](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.7).

---

## [0.1.6] - 2024-07-17

See [GitHub release](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.6).

---

## [0.1.5] - 2023-05-15

See [GitHub release](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.5).

---

## [0.1.2] - 2023-02-16

See [GitHub release](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.2).

---

## [0.1.1] - 2023-02-13

See [GitHub release](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.1).

---

## [0.1.0] - 2023-02-01

See [GitHub release](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.0).

---

## [0.0.6] - 2023-02-01

Initial pre-release.

---

## [0.0.5-alpha] - 2023-01-18

See [GitHub release](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.0.5-alpha).

Security fixes are identified explicitly in release notes and GitHub Security
Advisories when applicable.
