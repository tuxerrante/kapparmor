[![1. Create app](https://github.com/tuxerrante/kapparmor/actions/workflows/build-app.yml/badge.svg)](https://github.com/tuxerrante/kapparmor/actions/workflows/build-app.yml)
[![1. CodeQL](https://github.com/tuxerrante/kapparmor/actions/workflows/codeql.yml/badge.svg)](https://github.com/tuxerrante/kapparmor/actions/workflows/codeql.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/tuxerrante/kapparmor)](https://goreportcard.com/report/github.com/tuxerrante/kapparmor)
[![codecov](https://codecov.io/gh/tuxerrante/kapparmor/branch/main/graph/badge.svg?token=KVCU7EUBJE)](https://codecov.io/gh/tuxerrante/kapparmor) [![OpenSSF Best Practices](https://www.bestpractices.dev/projects/8391/badge)](https://www.bestpractices.dev/projects/8391)

# Kapparmor
- [Kapparmor](#kapparmor)
  - [Install](#install)
  - [Known limitations](#known-limitations)
  - [Testing](#testing)
  - [Release process](#release-process)
- [External useful links](#external-useful-links)
- -----
Apparmor-loader project to deploy profiles through a kubernetes daemonset.  


![architecture](./docs/kapparmor-architecture.png)

This app provide dynamic loading and unloading of [AppArmor profiles](https://ubuntu.com/server/docs/security-apparmor) to a Kubernetes cluster through a configmap.  
The app doesn't need an operator and it will be managed by a DaemonSet filtering the linux nodes to schedule the app pod.  
The custom profiles deployed in the configmap will be copied in a directory (`/etc/apparmor.d/custom` by default) since apparmor_parser needs the profiles definitions also to remove them. Once you will deploy a configmap with different profiles, Kapparmor will notice the missing ones and it will remove them from the apparmor cache and from the node directory.  
If you modify only the content of a profile leaving the same name, Kapparmor should notice it anyway since a byte comparison is done when configmap profiles names and local profiles names match.

1. The CD pipeline will
	- deploy a configmap in the security namespace containing all the profiles versioned in the current project
	- it will apply a daemonset on the linux nodes
2. The configmap will contain multiple apparmor profiles
    - The custom profiles names HAVE to start with the same PROFILE_NAME_PREFIX, currently this defaults to "custom.". 
    - The name of the file should be the same as the name of the profile.
3. The configmap will be polled every POLL_TIME seconds to move them into PROFILES_DIR host path and then enable them.

You can view which profiles are loaded on a node by checking the /sys/kernel/security/apparmor/profiles, so its parent will need to be mounted in the pod.

This work was inspired by [kubernetes/apparmor-loader](https://github.com/kubernetes/kubernetes/tree/master/test/images/apparmor-loader).


## Install
You can install the helm chart like this
```sh
helm repo add tuxerrante https://tuxerrante.github.io/kapparmor
helm upgrade kapparmor --install --atomic --timeout 120s --debug --set image.tag=pr-16 tuxerrante/kapparmor

```

## Known limitations
- Constraint: Profiles are validated on the `profile` keyword presence before of a opening curly bracket `{`.  
- Constraint: Profiles are validated on the `profile` keyword presence before of a opening curly bracket `{`.  
  It must be a [unattached profiles](https://documentation.suse.com/sles/15-SP1/html/SLES-all/cha-apparmor-profiles.html#sec-apparmor-profiles-types-unattached).
- Profile names have to start with `custom.` and to be equal to their filename.
- Profile names have to start with `custom.` and to be equal to their filename.
- There could be issues if you start the daemonsets on "dirty" nodes, where some old custom profiles were left after stopping or uninstalling Kapparmor.  
  E.G: By default if you delete a pod all the profiles should be automatically deleted from that node, but the app crashes during the process. 


## Testing
[There is a whole project meant to be a demo for this one](https://github.com/tuxerrante/kapparmor-demo), have fun.

Or you can find more info in [docs/testing.md](docs/testing.md)




## Release process
Commits and tags [should be signed](https://git-scm.com/book/en/v2/Git-Tools-Signing-Your-Work).  
Commits and tags [should be signed](https://git-scm.com/book/en/v2/Git-Tools-Signing-Your-Work).  
Update `config/config` file with the right app and chart version.  
Do the same in the chart manifest `charts/kapparmor/Chart.yaml`.  
Test it on a local cluster with `./build` scripts and following [docs/testing.md](docs/testing.md) instructions (go test, go lint, helm lint, helm template, helm install dry run...).  
Test it on a local cluster with `./build` scripts and following [docs/testing.md](docs/testing.md) instructions (go test, go lint, helm lint, helm template, helm install dry run...).  
Update the chart Changelog with the most relevant commits of this release, this will automatically fill the release page.  
Open the PR.  
Merge.  
Tag.  



# External useful links
- [KAppArmor Demo](https://github.com/tuxerrante/kapparmor-demo)
- [Kubernetes.io tutorials on apparmor](https://kubernetes.io/docs/tutorials/security/apparmor/)
- [Security Profiles Operator](https://github.com/kubernetes-sigs/security-profiles-operator/)
- [Kubernetes apparmor-loader](https://github.com/kubernetes/kubernetes/blob/master/test/images/apparmor-loader/loader.go)
