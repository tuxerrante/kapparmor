# Testing Guide

Kapparmor has two practical local integration paths:

- `test/e2e.py` plus the `make e2e*` targets for MicroK8s-based automation.
- Ubuntu Lima plus `k3s` for a single-node smoke test that exercises the real AppArmor host flow on Ubuntu.

Use the MicroK8s path when you want the existing scripted test cases. Use the Lima path when you want a production-like Ubuntu/AppArmor node and a fast way to validate DaemonSet behavior end-to-end.

## Quick Checks

```bash
# Unit tests
make test

# Coverage
make test-coverage

# Lint
make lint
```

## CI Container Integration

GitHub Actions already runs an Ubuntu-based container integration check in `.github/workflows/integration-test.yml`.
That job validates the container with AppArmor-capable host mounts, but it does not exercise the full Kubernetes DaemonSet path.
Use `./build/test_on_docker.sh` for the closest local equivalent.

## MicroK8s E2E

The built-in end-to-end harness is `test/e2e.py`. It is currently MicroK8s-specific because it shells out to `microk8s kubectl` and `microk8s ctr`.

Common entry points:

```bash
# Run all cases and build/side-load the image
make e2e-sideload

# Focused cases
make e2e-case1   # profile management
make e2e-case2   # in-use profile deletion
make e2e-case3   # metrics validation
```

Prerequisites:

- MicroK8s 1.30+
- Helm 3+
- Docker
- Python 3.9+

The harness already supports image side-loading and writes a timestamped log under `output/`.
If you need a MicroK8s environment inside a Linux VM, see `docs/microk8s.md`.

## Ubuntu Lima + k3s Smoke

This path is useful when you want to validate the real Ubuntu host integration:

- ConfigMap projected volume updates
- `apparmor_parser --replace` and `--remove`
- persisted files under `/etc/apparmor.d/custom`
- kernel state under `/sys/kernel/security/apparmor/profiles`
- pod metrics exposed on `/metrics`

It is a smoke test, not a replacement for the MicroK8s test harness.

### Tested Matrix

This smoke path was validated with the following environment:

| Layer | Validated setup |
| --- | --- |
| Host | macOS on Apple Silicon |
| Guest VM | Lima `template:ubuntu-24.04` |
| Guest architecture | `arm64` / `aarch64` |
| Cluster | single-node `k3s` |
| Built image | `linux/arm64` |

The commands below are written for that matrix first.

### Host vs Guest Scope

- `Host` means your local shell, where you run `limactl`, `docker`, and `helm`.
- `Guest` means commands executed inside the Ubuntu VM through `limactl shell "$VM" -- ...`.
- Unless noted otherwise, the snippets below are run on the host shell and invoke guest commands explicitly where needed.

### Requirements

- macOS with Lima installed
- Docker on the host
- Helm on the host
- Apple Silicon hosts should build `linux/arm64` images to match the default Lima VM architecture

### Linux amd64 Adaptation

If you are running on native Linux `amd64`, or on an Ubuntu `amd64` VM without Lima:

- skip the Lima-specific host setup and run the guest commands directly on that Linux machine;
- replace `limactl shell "$VM" -- bash -lc '<cmd>'` with either `bash -lc '<cmd>'` locally or the equivalent `ssh` wrapper for your VM;
- build `linux/amd64` images instead of `linux/arm64`, or omit `--platform` if the build host and Kubernetes node already match;
- keep the Helm values, ConfigMaps, and smoke assertions the same.

If you are on macOS Intel or any other `amd64` guest/node, the same rule applies: the image architecture must match the Kubernetes node architecture.

### Capture Results

To keep a full transcript of the smoke test, start a local typescript before running the commands:

```bash
mkdir -p output
script -q "output/lima-k3s-smoke-$(date -u +%Y%m%d_%H%M%S).log"
```

When the smoke test is finished, type `exit` to close the transcript.

### 1. Create the Ubuntu VM (run on host)

```bash
REPO=$(pwd)
VM=kapparmor-ubuntu2404

limactl start --name="$VM" template:ubuntu-24.04

limactl shell "$VM" -- bash -lc 'curl -sfL https://get.k3s.io | sh -'

limactl shell "$VM" -- bash -lc 'sudo k3s kubectl get nodes -o wide'
limactl shell "$VM" -- bash -lc 'cat /sys/module/apparmor/parameters/enabled && command -v apparmor_parser'
```

Expected result:

- the node is `Ready`
- `/sys/module/apparmor/parameters/enabled` prints `Y`
- `apparmor_parser` exists in the guest

Lima mounts the repo into the guest at the same absolute path, so the guest-side `kubectl apply -f` commands below can reuse `${REPO}`.

### 2. Build and Import the Test Image (run on host)

Run these commands from the repo root or from the worktree you want to validate:

```bash
APP_VERSION=$(awk -F= '/^APP_VERSION=/{print $2}' config/config)
APP_TAG="${APP_VERSION}-dev"

# Validated path: macOS Apple Silicon host -> Ubuntu arm64 guest
docker build --platform linux/arm64 \
  -t "ghcr.io/tuxerrante/kapparmor:${APP_TAG}" \
  --build-arg POLL_TIME=20 \
  --build-arg PROFILES_DIR=/app/profiles \
  -f Dockerfile \
  .

docker save "ghcr.io/tuxerrante/kapparmor:${APP_TAG}" | \
  limactl shell "$VM" -- bash -lc 'sudo k3s ctr images import /dev/stdin'
```

On Linux `amd64`, use `--platform linux/amd64` or omit `--platform` when the host and node architecture already match.

### 3. Deploy the Helm Chart Against the Local Image (run on host)

```bash
NS=security
GIT_SHA=$(git rev-parse --short=12 HEAD)
CHART_PATH="${REPO}/charts/kapparmor"

limactl shell "$VM" -- bash -lc \
  "sudo k3s kubectl create namespace ${NS} --dry-run=client -o yaml | sudo k3s kubectl apply -f -"

helm template kapparmor "${CHART_PATH}" \
  --namespace "${NS}" \
  --set image.pullPolicy=IfNotPresent \
  --set image.tag="${APP_TAG}" \
  --set service.enabled=true \
  --set "podAnnotations.gitCommit=${GIT_SHA}" | \
  limactl shell "$VM" -- bash -lc 'sudo k3s kubectl apply -f -'

limactl shell "$VM" -- bash -lc \
  "sudo k3s kubectl rollout status daemonset/kapparmor -n ${NS} --timeout=180s"

POD=$(limactl shell "$VM" -- bash -lc \
  "sudo k3s kubectl get pods -n ${NS} -l app.kubernetes.io/name=kapparmor -o jsonpath='{.items[0].metadata.name}'")
```

### 4. Useful Helpers (define on host shell)

The following helpers make the smoke test repeatable:

```bash
dump_metrics() {
  limactl shell "$VM" -- bash -lc "
    sudo k3s kubectl port-forward -n ${NS} pod/${POD} 18080:8080 >/tmp/kap-pf.log 2>&1 &
    PF=\$!
    sleep 3
    curl -fsS http://127.0.0.1:18080/metrics | grep '^kapparmor_'
    rc=\$?
    kill \$PF
    wait \$PF 2>/dev/null
    exit \$rc
  "
}

dump_host_state() {
  limactl shell "$VM" -- bash -lc "
    echo '--- FILE'
    if sudo test -f /etc/apparmor.d/custom/custom.deny-write-outside-home; then
      echo 'FILE_PRESENT true'
      sudo cat /etc/apparmor.d/custom/custom.deny-write-outside-home
    else
      echo 'FILE_PRESENT false'
    fi
    echo '--- KERNEL'
    if sudo grep -q 'custom.deny-write-outside-home' /sys/kernel/security/apparmor/profiles; then
      echo PRESENT
    else
      echo ABSENT
    fi
  "
}
```

### 5. Smoke Sequence (run on host shell)

Use the shipped ConfigMaps under `test/`:

- `test/cm-kapparmor-empty.yml`
- `test/cm-kapparmor-home-profile.yml`
- `test/cm-kapparmor-home-profile-edited.yml`

Recommended sequence:

```bash
# Baseline
limactl shell "$VM" -- bash -lc "sudo k3s kubectl apply -n ${NS} -f ${REPO}/test/cm-kapparmor-empty.yml"
sleep 35
dump_host_state
dump_metrics

# Create
limactl shell "$VM" -- bash -lc "sudo k3s kubectl apply -n ${NS} -f ${REPO}/test/cm-kapparmor-home-profile.yml"
sleep 45
dump_host_state
dump_metrics

# Modify
limactl shell "$VM" -- bash -lc "sudo k3s kubectl apply -n ${NS} -f ${REPO}/test/cm-kapparmor-home-profile-edited.yml"
sleep 45
dump_host_state
dump_metrics

# Delete
limactl shell "$VM" -- bash -lc "sudo k3s kubectl apply -n ${NS} -f ${REPO}/test/cm-kapparmor-empty.yml"
sleep 50
dump_host_state
dump_metrics
```

Expected pass criteria:

- Baseline:
  - no file under `/etc/apparmor.d/custom/custom.deny-write-outside-home`
  - kernel state is `ABSENT`
  - `kapparmor_profiles_managed` is `0`
- After create:
  - file exists on disk
  - kernel state is `PRESENT`
  - `kapparmor_profile_operations_total{operation="create",profile_name="custom.deny-write-outside-home"}` is `1`
  - `kapparmor_profiles_managed` is `1`
- After modify:
  - on-node file content reflects the edited ConfigMap
  - kernel state remains `PRESENT`
  - `modify` counter increments to `1`
  - `kapparmor_profiles_managed` stays `1`
- After delete:
  - file is removed from `/etc/apparmor.d/custom`
  - kernel state is `ABSENT`
  - `delete` counter increments to `1`
  - `kapparmor_profiles_managed` returns to `0`

### ConfigMap Convergence and First-Load Latency

Current behavior is polling-based.

- Kubernetes updates projected ConfigMap volumes asynchronously by rotating the `..data` symlink.
- Kapparmor notices those changes only when the next polling cycle reads `/app/profiles`.
- With the chart default `POLL_TIME=30`, the first observable reconcile after a ConfigMap change may take roughly one projected-volume refresh plus one poll interval. In practice, budget about `45-60s`.

If lower latency is required, the recommended implementation is:

1. run one eager reconcile before entering the periodic ticker, so profiles already present at pod startup load immediately;
2. add an `fsnotify` watch on `/app/profiles` and trigger reconcile when the projected volume swaps `..data` or profile symlinks;
3. keep the periodic ticker as a safety net in case a watch event is missed.

Shortening `POLL_TIME` reduces the worst-case delay, but by itself it does not make post-projection loads immediate.

### Cleanup

```bash
limactl shell "$VM" -- bash -lc 'sudo k3s kubectl delete namespace security --ignore-not-found'
limactl delete --force "$VM"
```

If you want to keep the VM for repeated smoke runs, skip the final `limactl delete` and just redeploy the namespace.

## Additional References

- `docs/microk8s.md` for a Linux VM MicroK8s setup
- `.github/workflows/integration-test.yml` for the Ubuntu-based container integration check used in CI
- `build/test_on_docker.sh` for the closest non-Kubernetes local equivalent
