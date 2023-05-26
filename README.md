[![1. Create app](https://github.com/tuxerrante/kapparmor/actions/workflows/build-app.yml/badge.svg)](https://github.com/tuxerrante/kapparmor/actions/workflows/build-app.yml)
[![1. CodeQL](https://github.com/tuxerrante/kapparmor/actions/workflows/codeql.yml/badge.svg)](https://github.com/tuxerrante/kapparmor/actions/workflows/codeql.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/tuxerrante/kapparmor)](https://goreportcard.com/report/github.com/tuxerrante/kapparmor)
[![codecov](https://codecov.io/gh/tuxerrante/kapparmor/branch/main/graph/badge.svg?token=KVCU7EUBJE)](https://codecov.io/gh/tuxerrante/kapparmor)

# Kapparmor
- [Kapparmor](#kapparmor)
  - [Features and constraints](#features-and-constraints)
  - [Install](#install)
  - [Known limitations](#known-limitations)
  - [Testing](#testing)
    - [How to initialize this project](#how-to-initialize-this-project)
    - [Test the app locally](#test-the-app-locally)
    - [Test on the Kubernetes cluster](#test-on-the-kubernetes-cluster)
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
- Constraint: Profiles are validated on the "`profile`" keyword presence before of a opening curly bracket `{`.  
  It must be a [unattached profiles](https://documentation.suse.com/sles/15-SP1/html/SLES-all/cha-apparmor-profiles.html#sec-apparmor-profiles-types-unattached).
- Profile names have to start with 'custom.' and to be equal as the filename containing it.
- There could be issues if you start the daemonsets on "dirty" nodes, where some old custom profiles were left after stopping or uninstalling Kapparmor.  
  E.G: By default if you delete a pod all the profiles should be automatically deleted from that node, but the app crashes during the process. 

- Not a limitation relative to this project, but if you deny write access in the /bin folder of a privileged container it could not be deleted by Kubernetes even after 'kubectl delete'. The command will succeed but the pod will stay in Terminating state.

## ToDo
- [X] Intercept Term signal and uninstall profiles before the Helm chart deletion completes.
- ⚠️ Implement the [controller-runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime#section-readme) design pattern through [Kubebuilder](https://book.kubebuilder.io/quick-start.html).
- 😁 Find funnier quotes for app starting and ending message (David Zucker, Monty Python, Woody Allen...).
- 🌱 Make the ticker loop thread safe: skip running a new loop if previous run is still ongoing.

## Testing
[There is a whole project meant to be a demo for this one](https://github.com/tuxerrante/kapparmor-demo), have fun.

Or you can find more info in [docs/testing.md](docs/testing.md)

# External useful links
- [KAppArmor Demo](https://github.com/tuxerrante/kapparmor-demo)
- [Kubernetes.io tutorials on apparmor](https://kubernetes.io/docs/tutorials/security/apparmor/)
- [Security Profiles Operator](https://github.com/kubernetes-sigs/security-profiles-operator/)
- [Kubernetes apparmor-loader](https://github.com/kubernetes/kubernetes/blob/master/test/images/apparmor-loader/loader.go)
