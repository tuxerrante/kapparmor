# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

1. **Unable to replace profiles**. Permission denied, app seems still confined.
1. Go unit tests  
    - [ ] Create a new profile
    - [ ] Update an existing profile
    - [ ] Remove an existing profile
    - [ ] Remove a non existing profile
1. Remove kubernetes Service and DaemonSet exposed ports if useless
1. Evaluate an automatic changelog generation from commits like [googleapis/release-please](https://github.com/googleapis/release-please)
1. Add daemonset commands for checking readiness
1. Add tests for all the main functions
1. Add test for checking current confinement state of the app
1. Test on multiple nodes cluster
1. helm diff in CD

## [0.0.6]() - 

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
