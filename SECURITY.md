# Security Policy

## Supported Versions

Currently only the latest versions of this project is being supported with security updates.

| Version | Supported          |
| ------- | ------------------ |
| Latest   | :white_check_mark: |

## Threat modeling
[Please refer to the **dedicated doc**](./docs/ThreatModel.md).  

From the documented self-assessment, a strong security posture emerges from the overall project architecture.

## Reporting a Vulnerability

This project leverages Snyk for automatic vulnerability detections and PRs opening.
Plus some code scannig tool is used during the CI build (Trivy, go vet) to block the build in case of high or critical vulnerabilities found.

If you want help or to propose a fix for a specific vulnerability, please use the Issues and Pull Requests sections as usual.
