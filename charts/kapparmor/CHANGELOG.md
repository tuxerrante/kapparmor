# Changelog

> All notable changes to this project will be documented in this file.  
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),  
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).


- 🚀 go 1.25
- Integration tests  
    - ✅ Create a new profile
    - ✅ Update an existing profile
    - ✅ Remove an existing profile
    - ✅ Check current confinement state of the app
- test_on_microk8s.sh - Main test script with:
  - ✅ Use helm chart approach
  - ✅ Fixed MicroK8s status check
  - ✅ Rebuilds image with --no-cache if missing
  - ✅ Adds build-time and gitCommit annotations
  - ❌ Skips RBAC
  - ✅ Implements two test cases
  - ✅ Shows logs and events in readable format
- 🌱 Switched to structured logging
- 🌱 Added different logging levels
- 🌱 Increased test coverage
- 🌱 **Liveness and Readiness server**
- 🌱 Filesystem writing operations protected by a **mutex**
- 🌱 Extensive integration testing bash automation 
- 🐞 Moved global vars to config struct
- 🐞 Removed shared signal channel. Moved to timeout based shutdown through context passing.
- 🐞 Removed panics to ensure cleanup and graceful shutdown
- 📖 Included **threat model analysis**.

### TODO:
- Generate signed OCI containers for all architectures
- Increase test coverage at least to 60%
- Implement [open telemetry](https://opentelemetry.io/docs/instrumentation/go/)
- Refactor code following [Google Go style guide](https://google.github.io/styleguide/go/guide)

---

## Previous Releases:
- [0.2.0](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.2.0) - 2024-02-19
- [0.1.5](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.5) - 2023-05-16
- [0.1.2](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.2) - 2023-02-16
- [0.1.1](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.1) - 2023-02-13
- [0.1.0](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.1.0) - 2023-02-01
- [0.0.6]() - 2023-01-26
- [0.0.5](https://github.com/tuxerrante/kapparmor/releases/tag/kapparmor-0.0.5-alpha) - 2023-01-23
