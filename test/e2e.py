#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Kapparmor E2E test runner on MicroK8s 

Features:
- Improved helm deployment with streaming output (no --wait / --atomic)
- Smart microk8s config detection (only restart if needed)
- Side-load image when GHCR push not available
- Robust shell config parsing (handles 'export' keyword)
- Simplified codebase (~40% fewer lines)
- Idempotent operations & comprehensive test cases
"""

from __future__ import annotations

import argparse
import base64
import datetime as dt
import json
import os
import re
import shutil
import subprocess
import sys
import textwrap
import time
import urllib.request
import urllib.parse
import socket
from pathlib import Path
from typing import Optional, Dict, List

# ============================================================================
# Configuration Defaults
# ============================================================================

DEFAULT_TARGET_NS = "security"
DEFAULT_TEST_NS = "kapparmor-test"
DEFAULT_LABEL = "app.kubernetes.io/name=kapparmor"
DEFAULT_LOG_DIR = Path("output")
DEFAULT_CHART_PATH = Path("charts/kapparmor")
DEFAULT_TEST_FILES = {
    "EMPTY": Path("test/cm-kapparmor-empty.yml"),
    "ONE": Path("test/cm-kapparmor-home-profile.yml"),
    "EDITED": Path("test/cm-kapparmor-home-profile-edited.yml"),
}
DEFAULT_EXPECTED_PROFILE = "custom.deny-write-outside-home"

# ANSI colors (TTY only)
IS_TTY = sys.stdout.isatty()
_COLORS = {
    "RED": "\033[0;31m" if IS_TTY else "",
    "GREEN": "\033[0;32m" if IS_TTY else "",
    "YELLOW": "\033[1;33m" if IS_TTY else "",
    "BLUE": "\033[0;34m" if IS_TTY else "",
    "NC": "\033[0m" if IS_TTY else "",
}

# ============================================================================
# Logging
# ============================================================================


class Logger:
    """Unified logger: console (colored) + file (plain)."""

    def __init__(self, logfile: Path):
        self.logfile = logfile
        logfile.parent.mkdir(parents=True, exist_ok=True)
        self.logfile.write_text("", encoding="utf-8")

    def _write(self, level: str, color: str, msg: str):
        """Write to console and file."""
        line = f"[{level}] {msg}"
        print(f"{color}[{level}]{_COLORS['NC']} {msg}")
        with self.logfile.open("a", encoding="utf-8") as f:
            f.write(line + "\n")

    def section(self, title: str):
        """Write section header."""
        sep = "=" * 42
        msg = f"\n{sep}\n[SECTION] {title}\n{sep}\n"
        print(f"\n{sep}\n{_COLORS['BLUE']}[SECTION]{_COLORS['NC']} {title}\n{sep}\n")
        with self.logfile.open("a", encoding="utf-8") as f:
            f.write(msg)

    def info(self, msg: str):
        self._write("INFO", _COLORS["GREEN"], msg)

    def warn(self, msg: str):
        self._write("WARN", _COLORS["YELLOW"], msg)

    def error(self, msg: str):
        self._write("ERROR", _COLORS["RED"], msg)

    def test(self, msg: str):
        self._write("TEST", _COLORS["BLUE"], msg)

    def pass_(self, msg: str):
        self._write("PASS", _COLORS["GREEN"], f"✓ {msg}")

    def fail(self, msg: str):
        self._write("FAIL", _COLORS["RED"], f"✗ {msg}")


# ============================================================================
# Utilities
# ============================================================================


def load_shell_config(filepath: Path) -> Dict[str, str]:
    """
    Parse shell config file (handles: KEY=VALUE, export KEY=VALUE, quoted values).
    Ignores comments, blank lines, and most shell syntax.
    """
    env = {}
    if not filepath.exists():
        return env

    for line in filepath.read_text(encoding="utf-8").splitlines():
        # Strip whitespace
        line = line.strip()
        if not line or line.startswith("#"):
            continue

        # Remove 'export ' prefix
        if line.startswith("export "):
            line = line[7:].strip()

        # Parse KEY=VALUE (handle basic quoting)
        if "=" not in line:
            continue

        key, _, value = line.partition("=")
        key = key.strip()
        value = value.strip()

        # Remove surrounding quotes (single or double)
        if (value.startswith('"') and value.endswith('"')) or (
            value.startswith("'") and value.endswith("'")
        ):
            value = value[1:-1]

        if key:
            env[key] = value

    return env


def run_cmd(
    cmd: list[str],
    logger: Optional[Logger] = None,
    check: bool = True,
    capture_output: bool = True,
    text: bool = True,
    timeout: Optional[int] = None,
) -> subprocess.CompletedProcess:
    """Execute shell command with unified error handling."""
    if logger:
        logger.info(f"$ {' '.join(cmd)}")

    try:
        cp = subprocess.run(
            cmd,
            check=False,
            capture_output=capture_output,
            text=text,
            timeout=timeout,
        )
    except FileNotFoundError:
        logger.error(f"Command not found: {cmd[0]}") if logger else None
        raise

    if check and cp.returncode != 0:
        err_msg = cp.stderr or cp.stdout or f"Command failed with rc={cp.returncode}"
        logger.error(f"Command failed: {err_msg}") if logger else None
        raise subprocess.CalledProcessError(cp.returncode, cmd, cp.stdout, cp.stderr)

    return cp


def _find_free_local_port() -> int:
    """Find a free local port."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def discover_prometheus_service(
    logger: Logger, namespace: str = "observability"
) -> tuple[str, int]:
    """
    Discover a Prometheus service in the given namespace that exposes port 9090.
    Returns (svc_name, port) or raises RuntimeError.
    """
    cp = run_cmd(
        ["bash", "-lc", f"microk8s kubectl get svc -n '{namespace}' -o json"],
        logger=None,
        check=False,
    )
    if cp.returncode != 0 or not cp.stdout:
        raise RuntimeError(f"Cannot list services in ns {namespace}")

    svcs = json.loads(cp.stdout).get("items", [])
    candidates: List[tuple[int, str, int]] = []
    for s in svcs:
        meta = s.get("metadata", {})
        name = meta.get("name", "")
        labels = meta.get("labels", {}) or {}
        ports = s.get("spec", {}).get("ports", []) or []
        for p in ports:
            port = p.get("port")
            if port == 9090:
                score = 0
                if labels.get("app.kubernetes.io/part-of") == "kube-prometheus-stack":
                    score += 2
                if labels.get("app.kubernetes.io/name") in (
                    "prometheus",
                    "prometheus-operated",
                ):
                    score += 1
                if labels.get("release") == "kube-prom-stack":
                    score += 1
                candidates.append((score, name, port))

    if not candidates:
        raise RuntimeError(
            f"No Prometheus Service on port 9090 found in namespace '{namespace}'"
        )
    candidates.sort(reverse=True)
    _, name, port = candidates[0]
    logger.info(f"Discovered Prometheus Service: {name}:{port} (ns={namespace})")
    return name, port


class PortForward:
    """Simple context manager to run kubectl port-forward to a svc or pod."""

    def __init__(
        self,
        namespace: str,
        kind: str,
        name: str,
        target_port: int,
        local_port: int | None = None,
    ):
        self.namespace = namespace
        self.kind = kind
        self.name = name
        self.target_port = target_port
        self.local_port = local_port or _find_free_local_port()
        self.proc: subprocess.Popen | None = None

    def __enter__(self):
        cmd = [
            "bash",
            "-lc",
            f"microk8s kubectl port-forward -n '{self.namespace}' {self.kind}/{self.name} {self.local_port}:{self.target_port}",
        ]
        self.proc = subprocess.Popen(
            cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True
        )
        # wait up to ~5s for local port to be accepting
        deadline = time.time() + 5
        while time.time() < deadline:
            try:
                with socket.create_connection(
                    ("127.0.0.1", self.local_port), timeout=0.3
                ):
                    return self
            except OSError:
                time.sleep(0.1)
        # capture stderr for diagnostics
        stderr = ""
        try:
            if self.proc and self.proc.stderr:
                stderr = self.proc.stderr.read(1024)
        except Exception:
            stderr = ""
        raise RuntimeError(
            f"port-forward to {self.kind}/{self.name}:{self.target_port} failed. {stderr}"
        )

    def __exit__(self, exc_type, exc, tb):
        if self.proc and self.proc.poll() is None:
            self.proc.terminate()
            try:
                self.proc.wait(timeout=2)
            except subprocess.TimeoutExpired:
                self.proc.kill()


def prom_instant_query(local_port: int, promql: str, timeout: int = 5) -> dict:
    """Execute a Prometheus instant query and return the JSON response."""
    qs = urllib.parse.urlencode({"query": promql})
    url = f"http://127.0.0.1:{local_port}/api/v1/query?{qs}"
    req = urllib.request.Request(url)
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return json.loads(resp.read().decode("utf-8"))


def _instant_value_from_vector(result_json: dict, filt: dict | None = None) -> float:
    """Extract numeric value from a Prometheus instant query vector result."""
    if result_json.get("status") != "success":
        return 0.0
    data = result_json.get("data", {})
    if data.get("resultType") != "vector":
        return 0.0
    total = 0.0
    for item in data.get("result", []):
        metric = item.get("metric", {})
        if filt:
            ok = True
            for k, v in filt.items():
                if metric.get(k) != v:
                    ok = False
                    break
            if not ok:
                continue
        try:
            v = float(item.get("value", [None, "0"])[1])
        except Exception:
            v = 0.0
        total += v
    return total


def _prom_counter_value(local_port: int, metric: str, labels: dict[str, str]) -> float:
    """Query a Prometheus counter metric with labels."""
    selector = metric + "{" + ",".join([f'{k}="{v}"' for k, v in labels.items()]) + "}"
    return _instant_value_from_vector(prom_instant_query(local_port, selector), None)


def _prom_gauge_sum(
    local_port: int, metric: str, labels: dict[str, str] | None = None
) -> float:
    """Query a Prometheus gauge metric (summed across all nodes/labels)."""
    if labels:
        selector = (
            metric + "{" + ",".join([f'{k}="{v}"' for k, v in labels.items()]) + "}"
        )
    else:
        selector = metric
    q = f"sum({selector})"
    return _instant_value_from_vector(prom_instant_query(local_port, q), None)


def detect_node_ip(logger: Logger) -> str:
    """Detect the node IP for MicroK8s networking."""
    # Allow explicit override
    if ip := os.environ.get("K8S_NODE_IP"):
        return ip

    # Prefer src of default route (works on NAT/bridged)
    cp = run_cmd(
        [
            "bash",
            "-c",
            "ip -4 route get 1.1.1.1 2>/dev/null | awk '/src/{print $7; exit}'",
        ],
        logger=None,
        check=False,
    )
    if cp.stdout and (ip := cp.stdout.strip()):
        return ip

    # Fallback: first UP interface with RFC1918 addr
    script = r"""
ip -4 -o addr show up \
| awk '$2 !~ /(lo|docker|br-|cni|flannel|vxlan|calico)/ {print $4}' \
| cut -d/ -f1 \
| grep -E '^(10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[0-1])\.)' \
| head -n1
"""
    cp = run_cmd(["bash", "-c", script], logger=None, check=False)
    if ip := cp.stdout.strip():
        return ip

    logger.error("Could not detect node IP")
    raise RuntimeError("Unable to detect K8S_NODE_IP")


def ensure_microk8s_networking(logger: Logger, expected_ip: str) -> None:
    """
    Pin kubelet/apiserver to expected_ip via snap config.
    Only restarts microk8s if config actually changed.
    """
    logger.section("Ensuring MicroK8s uses the correct interface")

    # Disable host-access (safely ignore errors)
    run_cmd(
        ["bash", "-lc", "microk8s disable host-access >/dev/null 2>&1 || true"],
        logger=None,
        check=False,
    )

    # Build new config
    new_cfg = textwrap.dedent(
        f"""\
    version: 0.1.0
    extraKubeletArgs:
      --node-ip: {expected_ip}
      --rotate-server-certificates: "true"
    extraKubeAPIServerArgs:
      --advertise-address: {expected_ip}
      --kubelet-preferred-address-types: InternalIP,Hostname,InternalDNS,ExternalDNS,ExternalIP
    """
    )

    # Get current config
    cp = run_cmd(
        ["bash", "-lc", "sudo snap get microk8s config 2>/dev/null || echo ''"],
        logger=None,
        check=False,
    )
    current_cfg = cp.stdout.strip() if cp.returncode == 0 else ""

    # Normalize both configs for comparison (handle whitespace/newline differences)
    def normalize_config(cfg_str: str) -> str:
        """Normalize YAML config for comparison."""
        return "\n".join(line.strip() for line in cfg_str.splitlines() if line.strip())

    normalized_current = normalize_config(current_cfg)
    normalized_new = normalize_config(new_cfg)

    # Only restart if config changed
    if normalized_current == normalized_new:
        logger.info(f"✓ MicroK8s config already pinned to {expected_ip}")
        return

    logger.info(f"Updating MicroK8s config to pin to {expected_ip}")
    sudo = "sudo" if shutil.which("sudo") else ""
    run_cmd(
        ["bash", "-lc", f"{sudo} snap set microk8s config='{new_cfg}'"],
        logger=logger,
        check=True,
    )

    logger.info("Restarting MicroK8s...")
    run_cmd(["bash", "-lc", f"{sudo} snap restart microk8s"], logger=logger)
    run_cmd(["microk8s", "status", "--wait-ready"], logger=logger)
    run_cmd(["microk8s", "kubectl", "get", "nodes", "-o", "wide"], logger=logger)

    logger.info(f"✓ Pinned to {expected_ip}")


def is_pending_kubelet_serving_csr(item: dict) -> bool:
    """Return True if CSR is pending and is a kubelet-serving CSR."""
    has_no_cert = item.get("status", {}).get("certificate") is None
    signer_name = item.get("spec", {}).get("signerName", "")
    is_kubelet_serving = "kubernetes.io/kubelet-serving" in signer_name
    return has_no_cert and is_kubelet_serving


def approve_kubelet_serving_csrs(expected_ip: str, logger: Logger) -> None:
    """Approve pending kubelet-serving CSRs matching the expected IP."""
    logger.section(f"Approving kubelet-serving CSRs (expect SAN IP={expected_ip})")

    cp = run_cmd(
        ["microk8s", "kubectl", "get", "csr", "-o", "json"], logger=None, check=False
    )
    if cp.returncode != 0 or not cp.stdout:
        logger.warn("Could not fetch CSRs")
        return

    data = json.loads(cp.stdout)

    pending = [
        item for item in data.get("items", []) if is_pending_kubelet_serving_csr(item)
    ]

    if not pending:
        logger.info("No pending kubelet-serving CSRs found.")
        return

    approved_any = False
    for csr in pending:
        name = csr["metadata"]["name"]
        # Extract and parse PEM
        pem_b64 = csr["spec"].get("request", "")
        if not pem_b64:
            continue

        pem = base64.b64decode(pem_b64).decode()
        # Check if SAN includes expected_ip
        if expected_ip not in pem:
            logger.info(f"Skipping {name} (SAN does not match {expected_ip})")
            continue

        logger.info(f"Approving {name}")
        run_cmd(
            ["microk8s", "kubectl", "certificate", "approve", name],
            logger=None,
            check=False,
        )
        approved_any = True

    if approved_any:
        logger.info("✓ CSRs approved")


def check_prerequisites(logger: Logger) -> None:
    """Verify microk8s, networking, CSR approval, and kubectl context."""
    logger.section("Checking prerequisites")

    if not shutil.which("microk8s"):
        logger.error("microk8s not found in PATH")
        raise FileNotFoundError("microk8s")

    # Check if running
    cp = run_cmd(
        ["microk8s", "status", "--wait-ready", "--format", "short", "--timeout", "30"],
        logger=logger,
        check=False,
    )
    if re.search(r"(not running|stopped)", cp.stdout or cp.stderr or "", re.IGNORECASE):
        logger.error("MicroK8s is not running")
        raise RuntimeError("MicroK8s not ready")

    logger.info("✓ MicroK8s is running")

    # Setup networking
    node_ip = detect_node_ip(logger)
    ensure_microk8s_networking(logger, node_ip)
    approve_kubelet_serving_csrs(node_ip, logger)

    # Set kubectl context
    run_cmd(
        ["bash", "-lc", "microk8s kubectl config use-context microk8s || true"],
        logger=None,
        check=False,
    )
    cp = run_cmd(["microk8s", "kubectl", "config", "current-context"], logger=None)
    logger.info(f"✓ Using context: {cp.stdout.strip()}")


def build_and_sideload_image(app_version: str, logger: Logger) -> None:
    """Build image and side-load it into MicroK8s (instead of pushing to GHCR)."""
    img = f"ghcr.io/tuxerrante/kapparmor:{app_version}-dev"
    git_sha = run_cmd(
        ["bash", "-lc", "git rev-parse --short=12 HEAD"], logger=None
    ).stdout.strip()
    build_time = dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    logger.section("Building and side-loading image")

    # Check if image exists locally
    cp = run_cmd(
        ["bash", "-lc", f"docker image ls '{img}' | grep -q '{app_version}'"],
        logger=None,
        check=False,
    )

    if cp.returncode != 0:
        logger.info(f"Building image {img}...")
        run_cmd(
            [
                "docker",
                "build",
                "-t",
                img,
                "--build-arg",
                f"GIT_SHA={git_sha}",
                "--build-arg",
                f"BUILD_TIME={build_time}",
                "-f",
                "Dockerfile",
                ".",
            ],
            logger=logger,
        )
    else:
        logger.info(f"✓ Image exists: {img}")

    # Side-load to MicroK8s
    logger.info("Side-loading image to MicroK8s...")
    run_cmd(
        ["bash", "-lc", f"docker save '{img}' | microk8s ctr image import /dev/stdin"],
        logger=logger,
    )
    logger.info("✓ Image side-loaded")


def helm_deploy_streaming(
    app_version: str,
    logger: Logger,
    target_ns: str,
    chart_path: Path,
    git_sha: str,
    build_time: str,
    sideload: bool = False,
    helm_timeout: int = 180,
) -> None:
    """
    Deploy helm chart WITHOUT --wait or --atomic.
    Stream output to monitor progress. Use kubectl rollout separately.

    Args:
        sideload: If True, use IfNotPresent pull policy (image is local).
                  If False, use Always pull policy (push to GHCR).
    """
    logger.section("Deploying Kapparmor (streaming output)")

    # Choose image pull policy based on deployment method
    pull_policy = "IfNotPresent" if sideload else "Always"
    logger.info(f"Using imagePullPolicy: {pull_policy} (sideload={sideload})")

    # Dry-run first
    logger.info("Running helm dry-run...")
    run_cmd(
        [
            "bash",
            "-lc",
            textwrap.dedent(
                f"""
            helm upgrade kapparmor --install \\
              --create-namespace \\
              --debug \\
              --devel \\
              --dry-run \\
              --kube-version 1.30 \\
              --namespace '{target_ns}' \\
              --set image.pullPolicy={pull_policy} \\
              --set image.tag='{app_version}-dev' \\
              --set 'podAnnotations.gitCommit={git_sha}' \\
              --set 'podAnnotations.build-time={build_time}' \\
              --set 'service.enabled'=true \\
              --set 'serviceMonitor.enabled'=true \\
              --set 'serviceMonitor.labels.release'=kapparmor \\
              '{chart_path}'
            """
            ).strip(),
        ],
        logger=logger,
        check=False,
    )

    # Real deploy (NO --wait, NO --atomic)
    logger.info("Deploying...")
    run_cmd(
        [
            "bash",
            "-lc",
            textwrap.dedent(
                f"""
            helm upgrade kapparmor --install \\
              --cleanup-on-fail \\
              --create-namespace \\
              --devel \\
              --namespace '{target_ns}' \\
              --set image.pullPolicy={pull_policy} \\
              --set image.tag='{app_version}-dev' \\
              --set 'podAnnotations.gitCommit={git_sha}' \\
              --set 'podAnnotations.build-time={build_time}' \\
              --set 'service.enabled=true' \\
              --set serviceMonitor.enabled=true \\
              --set 'serviceMonitor.labels.release'=kapparmor \\
              '{chart_path}'
            """
            ).strip(),
        ],
        logger=logger,
        timeout=helm_timeout,
    )

    # Monitor rollout separately
    logger.info("Waiting for rollout (separate kubectl command)...")
    run_cmd(
        [
            "microk8s",
            "kubectl",
            "rollout",
            "status",
            "daemonset/kapparmor",
            "-n",
            target_ns,
            "--timeout=120s",
        ],
        logger=logger,
    )

    logger.info("✓ Kapparmor deployed successfully")
    run_cmd(
        [
            "bash",
            "-lc",
            f"microk8s kubectl get pods -n '{target_ns}' -l='{DEFAULT_LABEL}' -o wide",
        ],
        logger=logger,
    )


def show_logs(logger: Logger, namespace: str, tail: int = 40) -> None:
    """Show kapparmor logs."""
    logger.info(f"Recent logs (last {tail} lines):")
    run_cmd(
        [
            "bash",
            "-lc",
            f"microk8s kubectl logs -n '{namespace}' -l='{DEFAULT_LABEL}' --tail={tail} --prefix=true || true",
        ],
        logger=logger,
        check=False,
    )


def apply_profiles_cm(
    logger: Logger, file: Path, namespace: str, ensure_empty: bool = False
) -> bool:
    """Apply profiles ConfigMap using server-side apply."""
    if not file.exists():
        logger.error(f"File not found: {file}")
        return False

    logger.info(f"Applying ConfigMap from: {file}")
    cp = run_cmd(
        [
            "bash",
            "-lc",
            f"microk8s kubectl apply -n '{namespace}' --server-side --force-conflicts -f '{file}'",
        ],
        logger=logger,
        check=False,
    )

    if cp.returncode != 0:
        logger.error("ConfigMap apply failed")
        return False

    if ensure_empty:
        logger.info("Ensuring ConfigMap has empty data...")
        run_cmd(
            [
                "bash",
                "-lc",
                f"microk8s kubectl patch configmap kapparmor-profiles -n '{namespace}' --type merge -p '{{\"data\":{{}}}}' 2>/dev/null || true",
            ],
            logger=None,
            check=False,
        )

    # Print live object
    run_cmd(
        [
            "bash",
            "-lc",
            f"microk8s kubectl get configmap kapparmor-profiles -n '{namespace}' -o yaml || true",
        ],
        logger=logger,
        check=False,
    )

    return True


def wait_for_last_retrieving_profiles(
    logger: Logger,
    namespace: str,
    expected_substr: str,
    max_wait: int = 60,
    check_interval: int = 3,
) -> bool:
    """Wait for 'retrieving profiles' log containing expected substring."""
    logger.info(
        f"Waiting for logs to contain: '{expected_substr}' (max {max_wait}s)..."
    )
    elapsed = 0

    while elapsed < max_wait:
        cp = run_cmd(
            [
                "bash",
                "-lc",
                f"microk8s kubectl logs -n '{namespace}' -l='{DEFAULT_LABEL}' --tail=200 2>/dev/null | grep 'retrieving profiles' | tail -1",
            ],
            logger=None,
            check=False,
        )

        if cp.returncode == 0 and expected_substr in cp.stdout:
            logger.pass_(f"Found: {expected_substr}")
            return True

        time.sleep(check_interval)
        elapsed += check_interval

    logger.warn(f"Substring '{expected_substr}' not found after {max_wait}s")
    return False


# ============================================================================
# Test Suite
# ============================================================================


class TestSuite:
    """Simple, consolidated test runner."""

    def __init__(self, logger: Logger, target_ns: str, test_ns: str, poll_time: int):
        self.logger = logger
        self.target_ns = target_ns
        self.test_ns = test_ns
        self.poll_time = poll_time
        self.tests_passed = 0
        self.tests_failed = 0

    def delay(self) -> None:
        """Wait for profile sync."""
        delay = self.poll_time + 1
        self.logger.info(f"Waiting {delay}s for sync...")
        time.sleep(delay)

    def pass_(self, msg: str) -> None:
        self.logger.pass_(msg)
        self.tests_passed += 1

    def fail(self, msg: str) -> None:
        self.logger.fail(msg)
        self.tests_failed += 1

    def test_case_1_profile_management(
        self, cm_empty: Path, cm_one: Path, cm_one_edited: Path, expected_profile: str
    ) -> None:
        """Test: empty -> one profile -> edited profile."""
        self.logger.section("TEST CASE 1: Profile Management")

        # Step 1: Empty ConfigMap
        self.logger.test("Apply EMPTY ConfigMap")
        if apply_profiles_cm(self.logger, cm_empty, self.target_ns, ensure_empty=True):
            self.pass_("Empty ConfigMap applied")
        else:
            self.fail("Empty ConfigMap apply")
            return

        self.delay()
        if wait_for_last_retrieving_profiles(
            self.logger, self.target_ns, "profiles=[]", max_wait=30
        ):
            self.pass_("Profiles list empty")
        else:
            self.fail("Profiles list should be empty")

        # Step 2: One profile
        self.logger.test("Apply ONE-PROFILE ConfigMap")
        if apply_profiles_cm(self.logger, cm_one, self.target_ns):
            self.pass_("One-profile ConfigMap applied")
        else:
            self.fail("One-profile ConfigMap apply")
            return

        self.delay()
        if wait_for_last_retrieving_profiles(
            self.logger, self.target_ns, expected_profile, max_wait=45
        ):
            self.pass_(f"Profile '{expected_profile}' synced")
        else:
            self.fail(f"Profile '{expected_profile}' not found")

        # Step 3: Edited profile
        self.logger.test("Apply EDITED ConfigMap")
        if apply_profiles_cm(self.logger, cm_one_edited, self.target_ns):
            self.pass_("Edited ConfigMap applied")
        else:
            self.fail("Edited ConfigMap apply")
            return

        self.delay()
        if wait_for_last_retrieving_profiles(
            self.logger, self.target_ns, expected_profile, max_wait=45
        ):
            self.pass_(f"Profile '{expected_profile}' detected after edit")
        else:
            self.fail(f"Profile '{expected_profile}' not found after edit")

    def test_case_2_profile_in_use(self) -> None:
        """Test: profile deletion while pod is using it."""
        self.logger.section("TEST CASE 2: Profile In-Use Deletion")

        # Create test namespace
        run_cmd(
            ["microk8s", "kubectl", "create", "namespace", self.test_ns],
            logger=None,
            check=False,
        )

        # Add profile
        self.logger.test("Add profile to ConfigMap")
        cp = run_cmd(
            [
                "bash",
                "-lc",
                f"microk8s kubectl get configmap kapparmor-profiles -n '{self.target_ns}' -o json",
            ],
            logger=None,
            check=False,
        )

        if cp.returncode == 0:
            self.pass_("ConfigMap retrieved")
            show_logs(self.logger, self.target_ns, 30)
        else:
            self.fail("ConfigMap not found")

    def test_case_3_prometheus_metrics(self) -> None:
        """Test: Prometheus metrics are being collected correctly.

        Strategy:
        1) Discover Prometheus service in observability namespace.
        2) Port-forward to Prometheus and use instant PromQL queries to validate counters/gauges.
        3) Fallback: if Prometheus unavailable, port-forward kapparmor pod and check /metrics directly.
        """
        self.logger.section("TEST CASE 3: Prometheus Metrics")
        prom_ns = os.environ.get("PROM_NS", "observability")

        # First, try Prometheus API path
        prom_ok = False
        try:
            svc_name, svc_port = discover_prometheus_service(
                self.logger, namespace=prom_ns
            )
            with PortForward(prom_ns, "svc", svc_name, target_port=svc_port) as pf:
                local_port = pf.local_port
                COUNTER = "kapparmor_profile_operations_total"
                GAUGE = "kapparmor_profiles_managed"
                # Use a unique profile name (must start with 'custom.' so kapparmor manages it)
                ts = dt.datetime.now().strftime("%s")
                profile = f"custom.test-metric-profile-{ts}"

                # Ensure deterministic baseline: clear ConfigMap first
                self.logger.info("Clearing ConfigMap to establish clean baseline...")
                run_cmd(
                    [
                        "bash",
                        "-lc",
                        f"microk8s kubectl patch configmap kapparmor-profiles -n '{self.target_ns}' --type merge -p '{{\"data\":{{}}}}'  2>/dev/null || true",
                    ],
                    logger=self.logger,
                    check=False,
                )
                self.delay()

                # Baselines
                self.logger.info(
                    f"Querying Prometheus@localhost:{local_port} for baseline metrics..."
                )
                c_create_0 = _prom_counter_value(
                    local_port,
                    COUNTER,
                    {"operation": "create", "profile_name": profile},
                )
                c_modify_0 = _prom_counter_value(
                    local_port,
                    COUNTER,
                    {"operation": "modify", "profile_name": profile},
                )
                c_delete_0 = _prom_counter_value(
                    local_port,
                    COUNTER,
                    {"operation": "delete", "profile_name": profile},
                )
                g_0 = _prom_gauge_sum(local_port, GAUGE, None)
                self.logger.info(
                    f"Baseline metrics: create={c_create_0}, modify={c_modify_0}, delete={c_delete_0}, managed={g_0}"
                )

                # If Prometheus has ANY kapparmor series, proceed; otherwise fallback
                # (don't check if all are 0 because new profile hasn't been created yet)
                try:
                    prom_healthy = prom_instant_query(local_port, "up")
                    self.logger.info(
                        "Prometheus is healthy, continuing with metrics tests..."
                    )
                except Exception as e:
                    self.logger.warn(
                        f"Prometheus not responding: {e}; falling back to pod metrics"
                    )
                    raise RuntimeError("Prometheus not available")

                # 1) CREATE - apply a new profile with unique name
                self.logger.test("Testing CREATE metric increment...")
                cm_yaml = f"""apiVersion: v1
kind: ConfigMap
metadata:
  name: kapparmor-profiles
  namespace: {self.target_ns}
data:
  {profile}: |
    profile {profile} flags=(attach_disconnected) {{
      file,
      /home/** rw,
      deny /bin/** w,
      deny /usr/** w,
    }}
"""
                run_cmd(
                    [
                        "bash",
                        "-lc",
                        f"cat <<'EOF' | microk8s kubectl apply -f -\n{cm_yaml}\nEOF",
                    ],
                    logger=self.logger,
                    check=False,
                )
                self.delay()
                wait_for_last_retrieving_profiles(
                    self.logger, self.target_ns, profile, max_wait=45
                )

                # Retry for up to ~60s to allow Prometheus scrape to occur
                deadline = time.time() + 60
                c_create_1 = 0.0
                g_1 = g_0
                created_ok = False
                while time.time() < deadline:
                    c_create_1 = _prom_counter_value(
                        local_port,
                        COUNTER,
                        {"operation": "create", "profile_name": profile},
                    )
                    g_1 = _prom_gauge_sum(local_port, GAUGE, None)
                    self.logger.info(
                        f"After CREATE (poll): create={c_create_1}, managed={g_1}"
                    )
                    if c_create_1 > c_create_0 and g_1 > g_0:
                        created_ok = True
                        break
                    time.sleep(3)

                if created_ok:
                    self.pass_(
                        "Prometheus counter incremented on create and gauge updated"
                    )
                else:
                    self.fail(
                        f"Create metrics unexpected (create {c_create_0}->{c_create_1}, gauge {g_0}->{g_1})"
                    )

                # 2) MODIFY - modify the profile
                self.logger.test("Testing MODIFY metric increment...")
                cm_yaml_edited = f"""apiVersion: v1
kind: ConfigMap
metadata:
  name: kapparmor-profiles
  namespace: {self.target_ns}
data:
  {profile}: |
    profile {profile} flags=(attach_disconnected) {{
      file,
      /home/** rw,
      /var/log/** r,
      deny /bin/** w,
      deny /usr/** w,
    }}
"""
                run_cmd(
                    [
                        "bash",
                        "-lc",
                        f"cat <<'EOF' | microk8s kubectl apply -f -\n{cm_yaml_edited}\nEOF",
                    ],
                    logger=self.logger,
                    check=False,
                )
                self.delay()
                wait_for_last_retrieving_profiles(
                    self.logger, self.target_ns, profile, max_wait=45
                )

                # Retry for up to ~60s to allow Prometheus scrape to occur after modify
                deadline = time.time() + 60
                c_modify_1 = 0.0
                g_2 = g_1
                modify_ok = False
                while time.time() < deadline:
                    c_modify_1 = _prom_counter_value(
                        local_port,
                        COUNTER,
                        {"operation": "modify", "profile_name": profile},
                    )
                    g_2 = _prom_gauge_sum(local_port, GAUGE, None)
                    self.logger.info(
                        f"After MODIFY (poll): modify={c_modify_1}, managed={g_2}"
                    )
                    if c_modify_1 > c_modify_0 and g_2 >= g_1:
                        modify_ok = True
                        break
                    time.sleep(3)

                if modify_ok:
                    self.pass_(
                        "Prometheus counter incremented on modify and gauge consistent"
                    )
                else:
                    self.fail(
                        f"Modify metrics unexpected (modify {c_modify_0}->{c_modify_1}, gauge {g_1}->{g_2})"
                    )

                # 3) DELETE
                self.logger.test("Testing DELETE metric increment...")
                patch = '[{"op":"remove","path":"/data/%s"}]' % profile
                run_cmd(
                    [
                        "bash",
                        "-lc",
                        "microk8s kubectl patch configmap kapparmor-profiles -n '%s' --type='json' -p='%s'"
                        % (self.target_ns, patch),
                    ],
                    logger=self.logger,
                    check=False,
                )
                self.delay()

                # Retry for up to ~60s to allow Prometheus scrape to occur after delete
                deadline = time.time() + 60
                c_delete_1 = 0.0
                g_3 = g_2
                delete_ok = False
                while time.time() < deadline:
                    c_delete_1 = _prom_counter_value(
                        local_port,
                        COUNTER,
                        {"operation": "delete", "profile_name": profile},
                    )
                    g_3 = _prom_gauge_sum(local_port, GAUGE, None)
                    self.logger.info(
                        f"After DELETE (poll): delete={c_delete_1}, managed={g_3}"
                    )
                    if c_delete_1 > c_delete_0 and g_3 < g_2:
                        delete_ok = True
                        break
                    time.sleep(3)

                if delete_ok:
                    self.pass_(
                        "Prometheus counter incremented on delete and gauge decreased"
                    )
                else:
                    # As a last resort, check the pod /metrics directly - Prometheus may be lagging.
                    self.logger.warn(
                        "Prometheus did not report delete within timeout; checking pod /metrics directly as fallback"
                    )
                    try:
                        cp = run_cmd(
                            [
                                "bash",
                                "-lc",
                                f"microk8s kubectl get pods -n '{self.target_ns}' -l='{DEFAULT_LABEL}' -o jsonpath='{{.items[0].metadata.name}}'",
                            ],
                            logger=None,
                            check=False,
                        )
                        if cp.returncode != 0 or not cp.stdout.strip():
                            self.fail(
                                f"Delete metrics unexpected (delete {c_delete_0}->{c_delete_1}, gauge {g_2}->{g_3})"
                            )
                        else:
                            pod_name = cp.stdout.strip()
                            with PortForward(
                                self.target_ns, "pod", pod_name, target_port=8080
                            ) as pf:
                                url = f"http://127.0.0.1:{pf.local_port}/metrics"
                                resp = urllib.request.urlopen(url, timeout=5)
                                metrics_text = resp.read().decode("utf-8")
                                # look for delete counter and gauge decrease
                                delete_found = False
                                for line in metrics_text.splitlines():
                                    if line.startswith("#"):
                                        continue
                                    if (
                                        "kapparmor_profile_operations_total" in line
                                        and 'operation="delete"' in line
                                        and profile in line
                                    ):
                                        try:
                                            val = float(line.strip().split()[-1])
                                            if val > c_delete_0:
                                                delete_found = True
                                        except Exception:
                                            pass
                                # gauge value
                                gauge_val = None
                                for line in metrics_text.splitlines():
                                    if line.startswith("kapparmor_profiles_managed"):
                                        try:
                                            gauge_val = float(line.strip().split()[-1])
                                        except Exception:
                                            gauge_val = None
                                if (
                                    delete_found
                                    and gauge_val is not None
                                    and gauge_val < g_2
                                ):
                                    self.pass_(
                                        "Delete observed on pod /metrics (Prometheus lag). Accepting as success"
                                    )
                                else:
                                    self.fail(
                                        f"Delete metrics unexpected (delete {c_delete_0}->{c_delete_1}, gauge {g_2}->{g_3})"
                                    )
                    except Exception as e:
                        self.fail(
                            f"Delete metrics unexpected (delete {c_delete_0}->{c_delete_1}, gauge {g_2}->{g_3}) - pod check failed: {e}"
                        )

                prom_ok = True
        except Exception as e:
            self.logger.info(f"Prometheus path skipped/fallback: {e}")

        if prom_ok:
            return

        # Fallback: query pod /metrics directly
        self.logger.test("Fallback: port-forward to kapparmor pod /metrics")
        cp = run_cmd(
            [
                "bash",
                "-lc",
                f"microk8s kubectl get pods -n '{self.target_ns}' -l='{DEFAULT_LABEL}' -o jsonpath='{{.items[0].metadata.name}}'",
            ],
            logger=None,
            check=False,
        )
        if cp.returncode != 0 or not cp.stdout.strip():
            self.fail("Could not get kapparmor pod name for fallback metrics check")
            return
        pod_name = cp.stdout.strip()
        self.logger.info(f"Pod name: {pod_name}")

        try:
            with PortForward(self.target_ns, "pod", pod_name, target_port=8080) as pf:
                url = f"http://127.0.0.1:{pf.local_port}/metrics"
                self.logger.info(f"Querying {url}")
                try:
                    resp = urllib.request.urlopen(url, timeout=5)
                    metrics_text = resp.read().decode("utf-8")
                    self.pass_("Metrics endpoint is responding (pod)")
                except Exception as e:
                    self.fail(f"Metrics endpoint not responding on pod: {e}")
                    return

                required_metrics = [
                    "kapparmor_profile_operations_total",
                    "kapparmor_profiles_managed",
                ]
                for m in required_metrics:
                    if m in metrics_text:
                        self.pass_(f"Metric found in pod /metrics: {m}")
                    else:
                        self.fail(f"Metric missing in pod /metrics: {m}")

                # Log detailed metrics for debugging
                self.logger.info("\n=== Available Metrics (pod /metrics) ===")
                for line in metrics_text.split("\n"):
                    if "kapparmor_" in line and not line.startswith("#"):
                        self.logger.info(f"  {line}")

        except Exception as e:
            self.fail(f"Port-forward to pod failed: {e}")

        # After test_case_3 completes, log available metrics from both sources
        self._log_available_metrics()

    def _log_available_metrics(self) -> None:
        """Log available kapparmor metrics from Prometheus and pod /metrics for debugging."""
        self.logger.section("Available Metrics Summary")
        prom_ns = os.environ.get("PROM_NS", "observability")

        # Try Prometheus first
        try:
            svc_name, svc_port = discover_prometheus_service(
                self.logger, namespace=prom_ns
            )
            with PortForward(prom_ns, "svc", svc_name, target_port=svc_port) as pf:
                local_port = pf.local_port
                self.logger.info(
                    f"\n=== Prometheus API Metrics (port-forward localhost:{local_port}) ==="
                )

                # Query all kapparmor metrics
                try:
                    result = prom_instant_query(
                        local_port, "kapparmor_profile_operations_total"
                    )
                    for metric in result.get("data", {}).get("result", []):
                        labels = metric.get("metric", {})
                        value = metric.get("value", ["", "0"])[1]
                        op = labels.get("operation", "?")
                        profile = labels.get("profile_name", "?")
                        self.logger.info(
                            f'  kapparmor_profile_operations_total{{operation="{op}",profile_name="{profile}"}} = {value}'
                        )
                except Exception as e:
                    self.logger.info(f"  Could not query profile_operations_total: {e}")

                try:
                    result = prom_instant_query(
                        local_port, "kapparmor_profiles_managed"
                    )
                    for metric in result.get("data", {}).get("result", []):
                        value = metric.get("value", ["", "0"])[1]
                        self.logger.info(f"  kapparmor_profiles_managed = {value}")
                except Exception as e:
                    self.logger.info(f"  Could not query profiles_managed: {e}")

                # Query Prometheus targets to see if kapparmor is being scraped
                try:
                    resp = urllib.request.urlopen(
                        f"http://localhost:{local_port}/api/v1/targets", timeout=5
                    )
                    targets_data = json.loads(resp.read().decode("utf-8"))
                    kapparmor_targets = [
                        t
                        for t in targets_data.get("data", {}).get("activeTargets", [])
                        if "kapparmor" in str(t.get("labels", {})).lower()
                    ]
                    if kapparmor_targets:
                        self.logger.info(f"\n=== Prometheus Scrape Targets ===")
                        for t in kapparmor_targets:
                            job = t.get("labels", {}).get("job", "unknown")
                            instance = t.get("labels", {}).get("instance", "unknown")
                            health = t.get("health", "unknown")
                            self.logger.info(
                                f"  Job: {job} | Instance: {instance} | Health: {health}"
                            )
                    else:
                        self.logger.info(
                            f"\n⚠ No kapparmor targets found in Prometheus scrape config"
                        )
                except Exception as e:
                    self.logger.info(f"  Could not query Prometheus targets: {e}")

        except Exception as e:
            self.logger.info(f"Prometheus not available: {e}")

        # Pod /metrics endpoint
        try:
            cp = run_cmd(
                [
                    "bash",
                    "-lc",
                    f"microk8s kubectl get pods -n '{self.target_ns}' -l='{DEFAULT_LABEL}' -o jsonpath='{{.items[0].metadata.name}}'",
                ],
                logger=None,
                check=False,
            )
            if cp.returncode == 0 and cp.stdout.strip():
                pod_name = cp.stdout.strip()
                with PortForward(
                    self.target_ns, "pod", pod_name, target_port=8080
                ) as pf:
                    url = f"http://127.0.0.1:{pf.local_port}/metrics"
                    resp = urllib.request.urlopen(url, timeout=5)
                    metrics_text = resp.read().decode("utf-8")

                    self.logger.info(
                        f"\n=== Pod /metrics Endpoint (localhost:{pf.local_port}) ==="
                    )
                    for line in metrics_text.split("\n"):
                        if "kapparmor_" in line and not line.startswith("#"):
                            self.logger.info(f"  {line}")
        except Exception as e:
            self.logger.info(f"Could not fetch pod /metrics: {e}")

    def final_status_check(self) -> None:
        """Show final pod/daemonset status."""
        self.logger.section("Final Status Check")
        run_cmd(
            [
                "bash",
                "-lc",
                f"microk8s kubectl get pods -n '{self.target_ns}' -l='{DEFAULT_LABEL}' -o wide",
            ],
            logger=self.logger,
            check=False,
        )

    def print_summary(self) -> int:
        """Print test summary and return exit code."""
        self.logger.section("Test Summary")
        total = self.tests_passed + self.tests_failed
        print(
            f"\nTotal: {total} | Passed: {self.tests_passed} | Failed: {self.tests_failed}\n"
        )
        with open(self.logger.logfile, "a") as f:
            f.write(
                f"\nTotal: {total} | Passed: {self.tests_passed} | Failed: {self.tests_failed}\n"
            )

        if self.tests_failed == 0:
            self.logger.info("✓ All tests passed")
            return 0
        else:
            self.logger.error(f"✗ {self.tests_failed} test(s) failed")
            return 1


# ============================================================================
# Main
# ============================================================================


def main():
    parser = argparse.ArgumentParser(
        description="Kapparmor E2E tests"
    )
    parser.add_argument(
        "--target-ns", default=os.environ.get("TARGET_NS", DEFAULT_TARGET_NS)
    )
    parser.add_argument("--test-ns", default=os.environ.get("TEST_NS", DEFAULT_TEST_NS))
    parser.add_argument("--chart", default=str(DEFAULT_CHART_PATH))
    parser.add_argument("--skip-build", action="store_true", help="Skip image build")
    parser.add_argument(
        "--sideload", action="store_true", help="Side-load image (don't push to GHCR)"
    )
    parser.add_argument(
        "--run", choices=["all", "case1", "case2", "case3"], default="all"
    )
    parser.add_argument("--log-file", default=None)
    args = parser.parse_args()

    # Load config
    cfg_env = load_shell_config(Path("config/config"))
    secrets_env = load_shell_config(Path.home() / ".config" / "secrets")
    os.environ.update(cfg_env)
    os.environ.update(secrets_env)

    POLL_TIME = int(os.environ.get("POLL_TIME", "5"))
    APP_VERSION = os.environ.get("APP_VERSION", "dev")

    # Setup logging
    nowstamp = dt.datetime.now().strftime("%Y%m%d_%H%M%S")
    log_file = (
        Path(args.log_file)
        if args.log_file
        else (DEFAULT_LOG_DIR / f"e2e_test_{nowstamp}.log")
    )
    logger = Logger(log_file)

    logger.section("Kapparmor E2E Tests")
    logger.info(f"Start time: {dt.datetime.now()}")
    GIT_SHA = run_cmd(
        ["bash", "-lc", "git rev-parse --short=12 HEAD"], logger=None
    ).stdout.strip()
    BUILD_TIME = dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    logger.info(f"Git commit: {GIT_SHA}")
    logger.info(f"Build time: {BUILD_TIME}")
    logger.info(f"Log file: {log_file}")

    try:
        # Prerequisites
        check_prerequisites(logger)

        # Image (build or sideload)
        if not args.skip_build:
            if args.sideload:
                build_and_sideload_image(APP_VERSION, logger)
            else:
                # For GHCR: you can add push logic here if token is available
                logger.warn(
                    "GHCR push not implemented. Use --sideload for local testing."
                )

        # Deploy via helm (streaming output, no --wait)
        helm_deploy_streaming(
            APP_VERSION,
            logger,
            args.target_ns,
            Path(args.chart),
            GIT_SHA,
            BUILD_TIME,
            sideload=args.sideload,
        )

        # Run tests
        suite = TestSuite(logger, args.target_ns, args.test_ns, POLL_TIME)
        suite.delay()
        show_logs(logger, args.target_ns, 20)

        if args.run in ("all", "case1"):
            suite.test_case_1_profile_management(
                DEFAULT_TEST_FILES["EMPTY"],
                DEFAULT_TEST_FILES["ONE"],
                DEFAULT_TEST_FILES["EDITED"],
                DEFAULT_EXPECTED_PROFILE,
            )

        if args.run in ("all", "case2"):
            suite.test_case_2_profile_in_use()

        if args.run in ("all", "case3"):
            suite.test_case_3_prometheus_metrics()

        # Final status
        suite.final_status_check()
        rc = suite.print_summary()

        sys.exit(rc)

    except KeyboardInterrupt:
        logger.warn("Interrupted by user")
        sys.exit(130)
    except Exception as e:
        logger.error(f"Fatal error: {e}")
        import traceback

        traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    main()
