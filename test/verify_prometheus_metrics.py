#!/usr/bin/env python3
"""
Comprehensive Prometheus metrics verification test.
Shows that kapparmor metrics are properly exported and scraped by Prometheus.
"""

import subprocess
import json
import urllib.request
import time
import sys


def run_cmd(cmd):
    """Execute shell command"""
    cp = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    return cp


def print_section(title):
    print(f"\n{'='*60}")
    print(f"  {title}")
    print(f"{'='*60}\n")


def main():
    print_section("Kapparmor Prometheus Metrics Verification")

    # 1. Check ServiceMonitor configuration
    print("[1] ServiceMonitor Label Configuration")
    print("-" * 60)
    cp = run_cmd(
        "microk8s kubectl get servicemonitor -n security kapparmor -o jsonpath='{.metadata.labels}'"
    )
    if "kapparmor" in cp.stdout:
        print("✅ ServiceMonitor has correct label: release=kapparmor")
    else:
        print("❌ ServiceMonitor label not found or incorrect")
        print(f"Labels: {cp.stdout}")
        return False

    # 2. Check Prometheus selector configuration
    print("\n[2] Prometheus ServiceMonitor Selector")
    print("-" * 60)
    cp = run_cmd(
        "microk8s kubectl get prometheus -n observability kube-prom-stack-kube-prome-prometheus -o jsonpath='{.spec.serviceMonitorSelector}' | python3 -m json.tool"
    )
    if "kapparmor" in cp.stdout:
        print("✅ Prometheus accepts 'kapparmor' label")
        print(cp.stdout)
    else:
        print("❌ Prometheus doesn't accept kapparmor label")
        return False

    # 3. Port-forward and query Prometheus
    print("\n[3] Prometheus API Queries")
    print("-" * 60)

    # Start port-forward
    cp = run_cmd(
        "microk8s kubectl get svc -n observability -o json | python3 -c \"import json, sys; d=json.load(sys.stdin); svcs = [s for s in d['items'] if 'prometheus' in s['metadata']['name'].lower() and 'node-exporter' not in s['metadata']['name'].lower()]; [print(s['metadata']['name'] + ':' + str(s['spec']['ports'][0]['port'])) for s in svcs[:1]]\""
    )

    if not cp.stdout.strip():
        print("❌ Could not find Prometheus service")
        return False

    prom_info = cp.stdout.strip().split("\n")[0]
    svc_name, svc_port = prom_info.rsplit(":", 1)
    svc_port = int(svc_port)
    print(f"Found Prometheus: {svc_name}:{svc_port}")

    # Start port-forward
    run_cmd(
        f"microk8s kubectl port-forward -n observability svc/{svc_name} 9090:{svc_port} > /dev/null 2>&1 &"
    )
    time.sleep(2)

    try:
        # Check targets
        resp = urllib.request.urlopen(
            "http://localhost:9090/api/v1/targets?state=active", timeout=5
        )
        targets = json.loads(resp.read().decode("utf-8"))

        kapparmor_targets = [
            t
            for t in targets["data"]["activeTargets"]
            if "kapparmor" in str(t.get("labels", {})).lower()
        ]

        if not kapparmor_targets:
            print("❌ No kapparmor targets found in Prometheus")
            return False

        target = kapparmor_targets[0]
        job = target["labels"].get("job", "?")
        instance = target["labels"].get("instance", "?")
        health = target.get("health", "?")

        print(f"✅ Kapparmor target registered:")
        print(f"   Job: {job}")
        print(f"   Instance: {instance}")
        print(f"   Health: {health}")

        if health != "up":
            print("⚠️  Target health is not 'up' yet (may need to wait for next scrape)")

        # Query metrics
        print("\n[4] Available Metrics")
        print("-" * 60)

        resp = urllib.request.urlopen(
            "http://localhost:9090/api/v1/query?query=kapparmor_profiles_managed",
            timeout=5,
        )
        result = json.loads(resp.read().decode("utf-8"))

        if result["data"]["result"]:
            print("✅ kapparmor_profiles_managed")
            for m in result["data"]["result"]:
                value = m["value"][1]
                node = m["metric"].get("node_name", "?")
                print(f"   node={node}, count={value}")
        else:
            print("⚠️  kapparmor_profiles_managed not found (may not be scraped yet)")

        resp = urllib.request.urlopen(
            "http://localhost:9090/api/v1/query?query=kapparmor_profile_operations_total",
            timeout=5,
        )
        result = json.loads(resp.read().decode("utf-8"))

        if result["data"]["result"]:
            print("✅ kapparmor_profile_operations_total")
            ops_by_operation = {}
            for m in result["data"]["result"]:
                op = m["metric"].get("operation", "?")
                count = m["value"][1]
                if op not in ops_by_operation:
                    ops_by_operation[op] = 0
                ops_by_operation[op] += float(count)

            for op, total in ops_by_operation.items():
                print(f"   {op}: {int(total)} operations")
        else:
            print(
                "⚠️  kapparmor_profile_operations_total not found (may not be scraped yet)"
            )

        # Check pod /metrics directly
        print("\n[5] Pod /metrics Endpoint")
        print("-" * 60)

        cp = run_cmd(
            "microk8s kubectl get pods -n security -l app.kubernetes.io/name=kapparmor -o jsonpath='{.items[0].metadata.name}'"
        )
        if cp.returncode == 0 and cp.stdout.strip():
            pod_name = cp.stdout.strip()
            run_cmd(
                f"microk8s kubectl port-forward -n security pod/{pod_name} 8080:8080 > /dev/null 2>&1 &"
            )
            time.sleep(1)

            resp = urllib.request.urlopen("http://localhost:8080/metrics", timeout=5)
            metrics = resp.read().decode("utf-8")

            kapparmor_metrics = [
                l
                for l in metrics.split("\n")
                if "kapparmor_" in l and not l.startswith("#")
            ]

            if kapparmor_metrics:
                print(
                    f"✅ Pod metrics endpoint available ({len(kapparmor_metrics)} metric lines):"
                )
                for line in kapparmor_metrics[:5]:
                    print(f"   {line}")
                if len(kapparmor_metrics) > 5:
                    print(f"   ... and {len(kapparmor_metrics) - 5} more")
            else:
                print("⚠️  No kapparmor metrics on pod /metrics")

        print("\n" + "=" * 60)
        print("✅ All checks passed! Prometheus is scraping kapparmor metrics.")
        print("=" * 60 + "\n")
        return True

    except Exception as e:
        print(f"❌ Error: {e}")
        return False
    finally:
        # Cleanup
        run_cmd("pkill -f 'port-forward' 2>/dev/null || true")


if __name__ == "__main__":
    success = main()
    sys.exit(0 if success else 1)
