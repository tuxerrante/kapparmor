You can find here also some indication on how to [set up a Microk8s environment in a Linux virtual machine](./microk8s.md).

[You can find HERE](https://gist.github.com/tuxerrante/9f9adf29405418427622b1e85d8c8263) instructions to fastly setup a devops dedicated ubuntu (virtual) machine.

### Requirements

```sh
sudo apt install yamllint
sudo snap install shfmt
go install github.com/yannh/kubeconform/cmd/kubeconform@latest

# Check all pre-commit hooks are installed
pre-commit run --config .pre-commit-config.yaml -v --hook-stage pre-commit --all-files
```

### How to initialize this project

```sh
helm create kapparmor
sudo usermod -aG docker $USER

# Create mod files in root dir
go mod init github.com/tuxerrante/kapparmor
go mod init ./src/app/
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
export IMAGE_TAG="0.1.6-dev"
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

## E2E Tests on MicroK8s

The E2E test suite validates KappArmor functionality end-to-end on a live Kubernetes cluster.

### Prerequisites

- MicroK8s 1.30+ (see `docs/microk8s.md` for setup)
- Helm 3.0+ (for chart deployment)
- Docker (for image building and side-loading)
- Python 3.9+ (with stdlib only, no external dependencies)
- Configuration files:
  - `./config/config` (APP_VERSION, POLL_TIME, etc.)
  - `$HOME/.config/secrets` (GH_WRITE_PKG_TOKEN, K8S_NODE_IP - optional)

### Quick Start

**Run all tests with image side-loading (no GHCR token needed):**

```sh
python3 test/e2e_refactored.py --sideload
```

**Run specific test case:**

```sh
# Profile management test (create/list/delete profiles)
python3 test/e2e_refactored.py --sideload --run case1

# In-use profile deletion test
python3 test/e2e_refactored.py --sideload --run case2
```

**Run with custom namespaces:**

```sh
python3 test/e2e_refactored.py --sideload \
  --target-ns security \
  --test-ns kapparmor-test
```

### Configuration

**Environment Variables** (can be set in `./config/config` or as env vars):

- `APP_VERSION`: Version string for the KappArmor app (default: read from config)
- `POLL_TIME`: Health check polling interval in seconds (default: 5)
- `TARGET_NS`: Namespace where KappArmor runs (default: `security`)
- `TEST_NS`: Namespace for test workloads (default: `kapparmor-test`)
- `HEALTHZPORT`: Port for health checks (default: 8080)

**Secrets** (optional, in `$HOME/.config/secrets`):

- `GH_WRITE_PKG_TOKEN`: GitHub token for pushing images to GHCR (if not using `--sideload`)
- `K8S_NODE_IP`: Node IP for MicroK8s networking configuration

### Command-Line Options

```sh
python3 test/e2e_refactored.py [OPTIONS]

Options:
  --target-ns NAMESPACE      Namespace for KappArmor DaemonSet (default: security)
  --test-ns NAMESPACE        Namespace for test workloads (default: kapparmor-test)
  --chart PATH              Path to Helm chart (default: charts/kapparmor)
  --skip-build              Skip Docker image build (use existing image)
  --sideload                Side-load image into MicroK8s (no GHCR token needed)
  --run {all,case1,case2,case3}  Which tests to run (default: all)
  --log-file PATH           Custom log file location (default: logs/e2e_test_TIMESTAMP.log)
  -h, --help               Show help message
```

### Test Cases

**Case 1: Profile Management**

- Creates ConfigMap with custom AppArmor profiles
- Verifies profiles are loaded into the system
- Lists loaded profiles to confirm deployment
- Deletes profiles via ConfigMap removal
- Verifies profiles are unloaded from the system

**Case 2: In-Use Profile Deletion**

- Creates and loads a restrictive profile
- Deploys a pod using the profile
- Attempts to delete the in-use profile
- Verifies profile remains (protected from deletion)
- Cleans up pod and then successfully deletes profile

**Case 3: Prometheus Metrics Validation**

- Validates that Prometheus metrics are exposed on `/metrics` endpoint
- Checks for required metrics:
  - `kapparmor_profile_operations_total`: Counter for create/modify/delete operations
  - `kapparmor_profiles_managed`: Gauge for currently managed profiles
- Attempts PromQL queries (if Prometheus is available)
- Verifies metrics are being collected and exposed correctly

### Troubleshooting

**Problem: "chart requires kubeVersion: >= 1.23.0-0"**

- **Solution**: Ensure your MicroK8s version is 1.23+

  ```sh
  microk8s version
  ```

**Problem: "Docker image not found"**

- **Solution**: Add `--skip-build` is only valid if image already exists

  ```sh
  docker image ls | grep kapparmor
  ```

**Problem: "Could not connect to GHCR / push failed"**

- **Solution**: Use `--sideload` flag to avoid pushing to registry

  ```sh
  python3 test/e2e_refactored.py --sideload
  ```

**Problem: "Config file not found"**

- **Solution**: Ensure `config/config` exists with required variables

  ```sh
  cat config/config  # Should have APP_VERSION, POLL_TIME, etc.
  ```

**Problem: "Metrics endpoint not responding" (Case 3)**

- **Solution**: Verify KappArmor pod is running and healthy

  ```sh
  microk8s kubectl get pods -n security -l app.kubernetes.io/name=kapparmor
  microk8s kubectl logs -n security -l app.kubernetes.io/name=kapparmor | grep metrics
  ```

- Note: The metrics endpoint is exposed on port 8080 by default (see HEALTHZPORT in config/config)

**Problem: "PromQL queries failing" (Case 3)**

- **Solution**: This is optional and only works if Prometheus is installed

  ```sh
  microk8s kubectl get svc -A | grep prometheus
  ```

- Test case will skip Prometheus checks if not available (non-blocking)

### CI/CD Integration

**GitHub Actions Example**:

```yaml
- name: Run E2E Tests
  run: |
    python3 test/e2e_refactored.py --sideload --run all
    
- name: Upload Logs
  if: always()
  uses: actions/upload-artifact@v4
  with:
    name: e2e-test-logs
    path: logs/e2e_test_*.log
```

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
