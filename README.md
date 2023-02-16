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
  - The custom profiles HAVE to start with the same PROFILE_NAME_PREFIX, currently this defaults to "custom.". 
  - The name of the file should be the same as the name of the profile.
3. The configmap will be polled every POLL_TIME seconds to move them into PROFILES_DIR host path and then enable them.

You can view which profiles are loaded on a node by checking the /sys/kernel/security/apparmor/profiles, so its parent will need to be mounted in the pod.

This work was inspired by [kubernetes/apparmor-loader](https://github.com/kubernetes/kubernetes/tree/master/test/images/apparmor-loader).

## Features and constraints
- Profile names have to start with 'custom.' and to be equal as the filename containing it

## Install
You can install the helm chart like this
```sh
helm repo add tuxerrante https://tuxerrante.github.io/kapparmor
helm upgrade kapparmor --install --atomic --timeout 100s --debug --set image.tag=dev tuxerrante/kapparmor

```

## Known limitations
- Profiles names are checked on the first line, so if there is some include before that would fail
- There could be issues if you start the daemonsets on "dirty" nodes, where some old custom profiles were left after stopping or uninstalling Kapparmor. E.g: you stop the pods and then redeploy the app with an empty profiles configmap without removing the previous custom profiles: Kapparmor will try to remove the old profiles but it could fail since there is no definition of them anymore.

## Testing
[Set up a Microk8s environment](./docs/microk8s.md).

### How to initialize this project
```sh
helm create kapparmor
sudo usermod -aG docker $USER

# Create mod files in root dir
go mod init github.com/tuxerrante/kapparmor
go mod init ./go/src/app/
```

### Test the app locally

Test Helm Chart creation
```sh
# --- Check the Helm chart
# https://github.com/helm/chart-testing/issues/464
echo Linting the Helm chart
helm lint --debug --strict  charts/kapparmor/

docker run -it --network host --workdir=/data --volume ~/.kube/config:/root/.kube/config:ro \
  --volume $(pwd):/data quay.io/helmpack/chart-testing:latest \
  /bin/sh -c "git config --global --add safe.directory /data; ct lint --print-config --charts ./charts/kapparmor"

# Replace here a commit id being part of an image tag
export GITHUB_SHA="sha-93d0dc4c597a8ae8a9febe1d68e674daf1fa919a"
helm install --dry-run --atomic --generate-name --timeout 30s --debug --set image.tag=$GITHUB_SHA  charts/kapparmor/

```

Test the app inside a container:
```sh
# --- Build and run the container image
docker build --quiet -t test-kapparmor --build-arg POLL_TIME=60 --build-arg PROFILES_DIR=/app/profiles -f Dockerfile . &&\
  echo &&\
  docker run --rm -it --privileged \
  --mount type=bind,source='/sys/kernel/security',target='/sys/kernel/security'  \
  --mount type=bind,source='/etc',target='/etc'\
  --name kapparmor  test-kapparmor

```

To test Helm chart installation in a MicroK8s cluster, follow docs/microk8s.md instructions if you don't have any local cluster.

### Test on the Kubernetes cluster
You can start a binary check inside the pod shell like this:
```sh
kapparmor_pod=$(kubectl get pods -l app.kubernetes.io/name=kapparmor --no-headers |grep Running |head -n1 |cut -d' ' -f1)
kubectl exec -it $kapparmor_pod -- cat /proc/1/attr/current
kubectl exec -it $kapparmor_pod -- cat /sys/module/apparmor/parameters/enabled
kubectl exec -it $kapparmor_pod -- cat /sys/kernel/security/apparmor/profiles |sort

# --- https://github.com/genuinetools/amicontained/releases
export AMICONTAINED_SHA256="d8c49e2cf44ee9668219acd092ed961fc1aa420a6e036e0822d7a31033776c9f"
curl -fSL "https://github.com/genuinetools/amicontained/releases/download/v0.4.9/amicontained-linux-amd64" -o "/usr/local/bin/amicontained" \
	&& echo "${AMICONTAINED_SHA256}  /usr/local/bin/amicontained" | sha256sum -c - \
	&& chmod a+x "/usr/local/bin/amicontained"
amicontained -h


```


# External useful links
- [https://emn178.github.io/online-tools/sha256.html](https://emn178.github.io/online-tools/sha256.html)
- [https://github.com/udhos/equalfile/blob/v0.3.0/equalfile.go](https://github.com/udhos/equalfile/blob/v0.3.0/equalfile.go)
- [https://github.com/kubernetes-sigs/security-profiles-operator/](https://github.com/kubernetes-sigs/security-profiles-operator/)
- [https://github.com/kubernetes/kubernetes/blob/master/test/images/apparmor-loader/loader.go](https://github.com/kubernetes/kubernetes/blob/master/test/images/apparmor-loader/loader.go)
