#!/usr/bin/env python3
"""Quick Prometheus metrics test - skip deployment, just test metrics"""

import subprocess
import sys
import time
import json
import urllib.request


def run_cmd(cmd):
    """Run shell command"""
    cp = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    return cp


def discover_prometheus():
    """Discover Prometheus service"""
    cp = run_cmd("microk8s kubectl get svc -n observability -o json")
    if cp.returncode != 0:
        return None
    data = json.loads(cp.stdout)
    for svc in data["items"]:
        name = svc["metadata"]["name"]
        if "prometheus" in name.lower():
            port = svc["spec"]["ports"][0]["port"]
            return name, port
    return None


def test_metrics():
    print("[INFO] Starting Prometheus metrics test...")

    # Discover Prometheus
    svc = discover_prometheus()
    if not svc:
        print("[FAIL] Could not find Prometheus service")
        return False

    svc_name, svc_port = svc
    print(f"[INFO] Found Prometheus: {svc_name}:{svc_port}")

    # Port-forward
    print(f"[INFO] Port-forwarding to Prometheus...")
    pf_cmd = f"microk8s kubectl port-forward -n observability svc/{svc_name} 9090:{svc_port} > /dev/null 2>&1 &"
    run_cmd(pf_cmd)
    time.sleep(3)

    try:
        # Query Prometheus
        print("\n[INFO] === Prometheus Targets ===")
        resp = urllib.request.urlopen(
            "http://localhost:9090/api/v1/targets?state=active", timeout=5
        )
        targets = json.loads(resp.read().decode("utf-8"))

        kapparmor_targets = [
            t
            for t in targets["data"]["activeTargets"]
            if "kapparmor" in str(t.get("labels", {})).lower()
        ]

        if kapparmor_targets:
            print(f"[PASS] Found {len(kapparmor_targets)} kapparmor target(s):")
            for t in kapparmor_targets:
                job = t["labels"].get("job", "?")
                instance = t["labels"].get("instance", "?")
                health = t.get("health", "?")
                print(f"       - Job: {job}, Instance: {instance}, Health: {health}")
        else:
            print("[WARN] No kapparmor targets found in Prometheus!")

        # Query metrics
        print("\n[INFO] === Prometheus Metrics ===")

        # Profile operations
        resp = urllib.request.urlopen(
            "http://localhost:9090/api/v1/query?query=kapparmor_profile_operations_total",
            timeout=5,
        )
        result = json.loads(resp.read().decode("utf-8"))

        ops = result["data"]["result"]
        if ops:
            print(f"[PASS] Found {len(ops)} profile operation metric(s):")
            for m in ops:
                labels = m["metric"]
                value = m["value"][1]
                op = labels.get("operation", "?")
                profile = labels.get("profile_name", "?")
                node = labels.get("node_name", "?")
                print(
                    f"       - op={op}, profile={profile}, node={node}, count={value}"
                )
        else:
            print("[INFO] No profile operations metrics yet")

        # Managed profiles
        resp = urllib.request.urlopen(
            "http://localhost:9090/api/v1/query?query=kapparmor_profiles_managed",
            timeout=5,
        )
        result = json.loads(resp.read().decode("utf-8"))

        managed = result["data"]["result"]
        if managed:
            print(f"[PASS] Found managed profiles metric:")
            for m in managed:
                value = m["value"][1]
                node = m["metric"].get("node_name", "?")
                print(f"       - node={node}, count={value}")
        else:
            print("[INFO] No managed profiles metric yet")

        # Pod /metrics
        print("\n[INFO] === Pod /metrics Endpoint ===")
        cp = run_cmd(
            "microk8s kubectl get pods -n security -l app.kubernetes.io/name=kapparmor -o jsonpath='{.items[0].metadata.name}'"
        )
        if cp.returncode == 0 and cp.stdout.strip():
            pod_name = cp.stdout.strip()
            pf_cmd = f"microk8s kubectl port-forward -n security pod/{pod_name} 8080:8080 > /dev/null 2>&1 &"
            run_cmd(pf_cmd)
            time.sleep(2)

            resp = urllib.request.urlopen("http://localhost:8080/metrics", timeout=5)
            metrics = resp.read().decode("utf-8")

            kapparmor_lines = [
                l
                for l in metrics.split("\n")
                if "kapparmor_" in l and not l.startswith("#")
            ]
            if kapparmor_lines:
                print(f"[PASS] Found {len(kapparmor_lines)} metric lines on pod:")
                for l in kapparmor_lines[:10]:  # Show first 10
                    print(f"       {l}")
            else:
                print("[WARN] No kapparmor metrics on pod /metrics!")

        return True

    except Exception as e:
        print(f"[FAIL] Error querying Prometheus: {e}")
        return False
    finally:
        # Cleanup port-forwards
        run_cmd("pkill -f 'port-forward' 2>/dev/null || true")


if __name__ == "__main__":
    success = test_metrics()
    sys.exit(0 if success else 1)
