# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## **0.1.7**
- Update to go 1.24.2

## [0.1.6 - 2024-07-16](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.6)
- Update to go 1.22.5
  - Security: fix [CVE-2024-24790](https://nvd.nist.gov/vuln/detail/CVE-2024-24790)
- Fixed produced image to ubuntu:24.04@sha256:2e863c44b718727c860746568e1d54afd13b2fa71b160f5cd9058fc436217b30
- Publication of a roadmap
- Docs update

## [0.1.5 - 2023-05-16](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.5)
- [Feature: manage custom labels](https://github.com/tuxerrante/kapparmor/commit/6e10b49720823930538cb9b86aa4a5f791efcb03)
- [Feature: validate profile file content](https://github.com/tuxerrante/kapparmor/commit/15da4e42893cdaa4412412a23c618ed98108714b)
- [Feature: Validate app and chart version](https://github.com/tuxerrante/kapparmor/commit/689fa391970cfd37a9c2410ebd860a3324b9fbd2)
- [Feature: catch SIGTERM signal](https://github.com/tuxerrante/kapparmor/commit/d8cc52cb7f62fa2f9995d56ef4c0a1008bb59203)
- [Fix: profile content checking when they have same name](https://github.com/tuxerrante/kapparmor/commit/5a97ba6071bbae2c75b28eb5969f8022d629afdd)
- [update to go 1.20](https://github.com/tuxerrante/kapparmor/commit/354ee4280d364057542b67df26dc75f96273b85c)
- Docs update

## [0.1.2](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.2) - 2023-02-22
### Fixed
- Support for profile names coming after comments and include lines
### Added
- Tested on multiple nodes cluster
- Base images switched to go 1.20



## [0.1.1](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.1) - 2023-02-13
### Fixed
- Moved shared testing functions to a dedicated module
- Minor documentation and readme fixes
### Added
- Enforce profiles filenames to be the same as the profile names
- Changelog automatically read by chart-releaser


## [0.1.0](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.0) - 2023-02-01
### Fixed
1. "Unable to replace profiles. Permission denied, app seems still confined." - Switched to ubuntu image
1. No need for SYS_ADMIN capabilities
1. Ignore hidden and system folders while scanning for profiles

### Added
1. Instructions to test the app in a virtual machine directly running the go app or in microk8s pushing the built container to the local registry


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
