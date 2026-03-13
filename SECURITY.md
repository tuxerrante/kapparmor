# Security Policy

## Supported Versions

Only the latest release of this project receives security updates.

| Version | Supported          |
| ------- | ------------------ |
| Latest  | :white_check_mark: |
| Older   | :x:                |

## Threat Modeling

[Please refer to the **dedicated doc**](./docs/ThreatModel.md).

From the documented self-assessment, a strong security posture emerges from the overall project architecture.

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

### Private Disclosure (preferred)

Use [GitHub Private Security Advisories](https://github.com/tuxerrante/kapparmor/security/advisories/new) to report a vulnerability privately. This keeps the details confidential until a fix is available.

When reporting, please include:
- A description of the vulnerability and its potential impact
- Steps to reproduce or a proof-of-concept
- Any suggested mitigations or fixes (optional but welcome)

### Response Timeline

| Activity | Target |
|---|---|
| Initial acknowledgement | Within **7 days** |
| Triage and severity assessment | Within **14 days** |
| Fix released (critical/high) | Within **90 days** |
| Public disclosure | After fix is available, coordinated with reporter |

We follow responsible disclosure: once a fix is published, the advisory will be made public and the reporter credited (if they wish).

### Automated Vulnerability Detection

This project uses the following tools for ongoing vulnerability monitoring:

- **Snyk** – automatic vulnerability detection and automated fix PRs
- **Trivy** – container image scanning in CI (blocks on CRITICAL/HIGH findings)
- **CodeQL** – static analysis in CI
- **Dependabot** – weekly dependency update PRs
- **Gosec** – Go security analysis in CI
