# Roadmap
## [Unreleased]

1. E2E tests  
    - [ ] Create a new profile
    - [ ] Update an existing profile
    - [ ] Remove an existing profile
    - [ ] Remove a non existing profile
    - [ ] check current confinement state of the app
2. Remove kubernetes Service and DaemonSet exposed ports if useless
4. Add daemonset commands for checking readiness
7. Add different logging levels
9. 🌱 Make the ticker loop thread safe: skip running a new loop if previous run is still ongoing.
8. ❓ Implement the [controller-runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime#section-readme) design pattern through [Kubebuilder](https://book.kubebuilder.io/quick-start.html).
   - Read again [Operator-WhitePaper/designing-operators](https://github.com/cncf/tag-app-delivery/blob/main/operator-wg/whitepaper/Operator-WhitePaper_v1-0.md#designing-operators)

