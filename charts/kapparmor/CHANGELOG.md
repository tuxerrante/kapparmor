# Changelog

> All notable changes to this project will be documented in this file.  
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),  
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

- E2E tests  
    - [ ] Create a new profile
    - [ ] Update an existing profile
    - [ ] Remove an existing profile
    - [ ] Remove a non existing profile
    - [ ] check current confinement state of the app
- Add different logging levels
- Generate signed OCI containers for all architectures
- Increase test coverage at least to 60%
- Implement [open telemetry](https://opentelemetry.io/docs/instrumentation/go/)
- Refactor directories similarly to [kubernetes-sigs](https://github.com/kubernetes-sigs) structure (eg: go/kapparmor/app/*.go) or to this [golang standard project layout](https://github.com/golang-standards/project-layout)
- Refactor code following [Google Go style guide](https://google.github.io/styleguide/go/guide)
- Move global vars to structs passed by reference
- Drop [Linux capabilities](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/#set-capabilities-for-a-container)
- Restrict syscalls through [seccomp](https://kubernetes.io/docs/tutorials/security/seccomp/#create-a-pod-that-uses-the-container-runtime-default-seccomp-profile) ([default profile](https://docs.docker.com/engine/security/seccomp/#significant-syscalls-blocked-by-the-default-profile))

---

## [0.2.0 - 2024-02-19](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.2.0)
CI:
- [X] Fixed Codecov plugin issues
- [X] Refresh container image every Sunday night
- [X] Git auto CRLF set to false `git config --global core.autocrlf false`
- [X] Bumped multiple actions
- [X] Bash CI to automate go version bump from one source of truth (`config/config`)

Code:
- [X] golang:1.22 [release notes](https://go.dev/doc/go1.22)
- [X] The k8s service resource is now settable from the values.yaml
- [X] Introduced Fuzz testing for profile filenames
- [X] If POLL_TIME is set less than 1 it will default to 1 second

Project Security Fixes
- [X] [Signed commits](https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-commits): `git config commit.gpgsign true`
- [X] Added repository [Security policy](https://github.com/tuxerrante/kapparmor/blob/main/SECURITY.md)
- [X] Added OpenSSF scorecard workflow
- [X] Least Privileged GitHub Actions Token Permissions: setting minimum token permissions for the GITHUB_TOKEN
- [X] Pinning actions to full length commit
- [X] Intergated [Harden-Runner](https://github.com/step-security/harden-runner) in the CI: it prevents exfiltration of credentials, detects tampering of source code during build, and enables running jobs without sudo access.
- [X] Pinned image tags to digests in Dockerfiles.
- [X] Closed 44 (!) security issues coming from [Scorecard security scanner](https://github.com/marketplace/actions/ossf-scorecard-action). Also with the help of [stepsecurity.io](https://app.stepsecurity.io/)

---

## [0.1.5 - 2023-05-16](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.5)
- [Feature: manage custom labels](https://github.com/tuxerrante/kapparmor/commit/6e10b49720823930538cb9b86aa4a5f791efcb03)
- [Feature: validate profile file content](https://github.com/tuxerrante/kapparmor/commit/15da4e42893cdaa4412412a23c618ed98108714b)
- [Feature: Validate app and chart version](https://github.com/tuxerrante/kapparmor/commit/689fa391970cfd37a9c2410ebd860a3324b9fbd2)
- [Feature: catch SIGTERM signal](https://github.com/tuxerrante/kapparmor/commit/d8cc52cb7f62fa2f9995d56ef4c0a1008bb59203)
- [Fix: profile content checking when they have same name](https://github.com/tuxerrante/kapparmor/commit/5a97ba6071bbae2c75b28eb5969f8022d629afdd)
- [update to go 1.20](https://github.com/tuxerrante/kapparmor/commit/354ee4280d364057542b67df26dc75f96273b85c)
- Docs update


---
## [0.1.2]() - 2023-02-22
### Fixed
- Support for profile names coming after comments and include lines
### Added
- Tested on multiple nodes cluster
- Base images switched to go 1.20

---
## [0.1.1]() - 2023-02-13
### Fixed
- Moved shared testing functions to a dedicated module
- Minor documentation and readme fixes
### Added
- Enforce profiles filenames to be the same as the profile names
- Changelog automatically read by chart-releaser

---
## [0.1.0](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.0) - 2023-02-01
### Fixed
1. "Unable to replace profiles. Permission denied, app seems still confined." - Switched to ubuntu image
1. No need for SYS_ADMIN capabilities 
1. Ignore hidden and system folders while scanning for profiles

### Added
1. Instructions to test the app in a virtual machine directly running the go app or in microk8s pushing the built container to the local registry

---
## 0.0.6 - 2023-01-26

### Added 
Helm:
- Added SYS_ADMIN capabilities to the daemonset
- Mounted needed folders in the Dockerfile and in the daemonset
- Added POLL_TIME and profiles files as configurable options through configmaps

Go:
- Added first testing function
- Moved file operations functions to dedicated module
  - Fixed POLL_TIME value passing from configmap

CI/CD:
- Explicit changelog to help users understanding the project features
  - Automatic generation of release notes based on changelog file
- Configurable poll time and profiles directory in the helm values file
---
## [0.0.5](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.0.5-alpha) - 2023-01-23

### Added 

Helm:
- Helm Chart based mainly on a DaemonSet and a configmap. No operator needed.
- Load all AppArmor profiles in the configmap template

Go:
- Possibility to load continuously the security profiles from a configmap with a configurable poll time

CI/CD:
- Helm chart linting and testing before releasing
- Security vulnerability tests on Go dependencies and container file.
- Auto generation of [GitHub pages](https://tuxerrante.github.io/kapparmor/)
- Container image tag is set to current commit SHA for every release. 

### Fixed

- Being still an alpha release I will add everything in the "Added" section
