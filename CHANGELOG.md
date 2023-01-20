# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
1. Go unit tests  
    - [ ] Create a new profile
    - [ ] Update an existing profile
    - [ ] Remove an existing profile
    - [ ] Remove a non existing profile
1. Remove kubernetes Service and DaemonSet exposed ports if useless
1. Evaluate an automatic changelog generation from commits like [googleapis/release-please](https://github.com/googleapis/release-please)

## [0.0.6]() - 

### Added 

Helm:

Go:

CI/CD:
- Explicit changelog to help users understanding the project features
  - Automatic generation of release notes based on changelog file


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
