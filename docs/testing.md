You can find here also some indication on how to [set up a Microk8s environment in a Linux virtual machine](./microk8s.md).

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
export IMAGE_TAG="0.1.6_dev"
helm upgrade kapparmor --install --dry-run \
    --atomic \
    --timeout 30s \
    --debug \
    --namespace test \
    --set image.tag=$IMAGE_TAG  charts/kapparmor/

```

Test the app inside a container:
```sh
# --- Build and run the container image
docker build --quiet -t test \
  --build-arg POLL_TIME=5 \
  --build-arg PROFILES_DIR=/app/profiles \
  -f Dockerfile.dev . &&\
  echo &&\
  docker run --rm -it --privileged \
  --mount type=bind,source='/sys/kernel/security',target='/sys/kernel/security'  \
  --mount type=bind,source='/etc',target='/etc' \
  --mount type=bind,source='/sbin',target='/sbin' \
  --name kapparmor test

```

To test Helm chart installation in a MicroK8s cluster, follow `docs/microk8s.md` instructions if you don't have any local cluster.

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
