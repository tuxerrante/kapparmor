#!/bin/bash
# Comprehensive MicroK8s test script for Kapparmor
# Tests profile management, pod enforcement, and edge cases

set -euo pipefail

# ----- Configuration
# --- Colors (for TTY only) ---
if [[ -t 1 ]]; then
	RED=$'\033[0;31m'
	GREEN=$'\033[0;32m'
	YELLOW=$'\033[1;33m'
	BLUE=$'\033[0;34m'
	NC=$'\033[0m'
else
	RED=""
	GREEN=""
	YELLOW=""
	BLUE=""
	NC=""
fi

. ./config/config
. $HOME/.config/secrets
# . ./config/secrets

readonly DELAY_SECONDS=$((POLL_TIME + 1))
readonly GIT_SHA=$(git rev-parse --short=12 HEAD)
readonly BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
readonly TARGET_NS="security"
readonly TEST_NS="kapparmor-test"
readonly LOG_FILE="output/microk8s_test_$(date +%Y%m%d_%H%M%S).log"

TESTS_PASSED=0
TESTS_FAILED=0

# ==========================================
# Logging Functions
# ==========================================

# Strip ANSI helper
_strip_ansi() { sed -r 's/\x1B\[[0-9;]*[mK]//g'; }

# Core logger
__log() {
	local level="$1"
	shift
	local color="$1"
	shift
	local msg="$*"
	# to screen (colored if TTY)
	printf "%b[%s]%b %s\n" "${color}" "${level}" "${NC}" "${msg}"
	# to file (color stripped)
	printf "[%s] %s\n" "${level}" "$(printf "%s" "${msg}" | _strip_ansi)" >>"$LOG_FILE"
}

log_info() { __log "INFO" "$GREEN" "$*"; }
log_warn() { __log "WARN" "$YELLOW" "$*"; }
log_error() { __log "ERROR" "$RED" "$*"; }
log_test() { __log "TEST" "$BLUE" "$*"; }
log_section() {
	local line="=========================================="
	printf "\n%s\n" "$line" | tee -a "$LOG_FILE"
	__log "SECTION" "$BLUE" "$*"
	printf "\n%s\n" "$line" | tee -a "$LOG_FILE"
}

test_passed() {
	__log "PASS" "$GREEN" "✓ $*"
	((++TESTS_PASSED))
}
test_failed() {
	__log "FAIL" "$RED" "✗ $*"
	((++TESTS_FAILED))
}

# Redirect all process stderr to the log (and screen)
# - If TTY: duplicate to screen via tee; otherwise append only to file
if [[ -t 2 ]]; then
	exec 2> >(stdbuf -oL tee -a "$LOG_FILE" >&2)
else
	exec 2>>"$LOG_FILE"
fi

# ==========================================
# Cleanup Function
# ==========================================

cleanup() {
	log_info "Cleaning up test resources..."

	# Delete test pods
	microk8s kubectl delete pod busy-dontwrite -n "$TEST_NS" --ignore-not-found=true --grace-period=0 --wait=false || true
	microk8s kubectl delete pod ubuntu-custom-profile -n "$TEST_NS" --ignore-not-found=true --grace-period=0 --wait=false || true

	# Delete test namespace
	microk8s kubectl delete namespace "$TEST_NS" --ignore-not-found=true --wait=false || true

	# Helm release
	# helm uninstall kapparmor -n "$TARGET_NS" --wait >/dev/null 2>&1 || true

	log_info "Cleanup complete (namespaces will be deleted in background)"
}

trap cleanup EXIT

# ==========================================
# Prerequisites Check
# ==========================================

# --- MicroK8s networking bootstrap ---
detect_node_ip() {
	# Allow explicit override
	if [[ -n ${K8S_NODE_IP:-} ]]; then
		echo "$K8S_NODE_IP"
		return
	fi
	# Prefer the src of default route (works on NAT/bridged)
	local src
	src=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '/src/{print $7; exit}')
	if [[ -n $src ]]; then
		echo "$src"
		return
	fi
	# Fallback: first UP interface with RFC1918 addr, excluding docker/cni/calico/lo
	ip -4 -o addr show up | awk '$2 !~ /(lo|docker|br-|cni|flannel|vxlan|calico)/ {print $4}' |
		cut -d/ -f1 | grep -E '^(10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[0-1])\.)' | head -n1
}

# Returns list of pending kubelet-serving CSR names (jq path).
_get_pending_serving_csrs_jq() {
	microk8s kubectl get csr -o json |
		jq -r '
      [.items[]
        | select(.spec.signerName=="kubernetes.io/kubelet-serving")
        | select(
            # Pending if there are no conditions or there is no "Approved" condition
            (.status.conditions | not)
            or
            ((.status.conditions | map(.type) | index("Approved")) | not)
          )
        | .metadata.name
      ] | .[]'
}

# Fallback without jq: parse kubectl table (NAME ... SIGNERNAME ... CONDITION)
_get_pending_serving_csrs_table() {
	microk8s kubectl get csr --no-headers 2>/dev/null |
		awk '$0 ~ /kubernetes\.io\/kubelet-serving/ && $NF=="Pending" {print $1}'
}

# Approve only kubelet-serving CSRs whose SAN includes the expected IP
approve_kubelet_serving_csr() {
	local ip="$1"
	log_section "Approving kubelet serving CSR (SAN must include ${ip})"

	local csrs=""
	if command -v jq >/dev/null 2>&1; then
		csrs=$(_get_pending_serving_csrs_jq) || true
	else
		csrs=$(_get_pending_serving_csrs_table) || true
	fi

	if [[ -z $csrs ]]; then
		log_info "No pending kubelet-serving CSRs found."
		return 0
	fi

	local approved_any=0
	while read -r name; do
		[[ -z $name ]] && continue
		# Extract CSR PEM and read SANs
		local pem san
		pem=$(microk8s kubectl get csr "$name" -o jsonpath='{.spec.request}' | base64 -d)
		san=$(printf "%s" "$pem" | openssl req -noout -text 2>/dev/null | awk '/Subject Alternative Name/{flag=1;next}/Attributes/{flag=0}flag')

		if printf "%s" "$san" | grep -q "IP Address:${ip}"; then
			log_info "Approving ${name} (SAN OK: ${ip})"
			microk8s kubectl certificate approve "$name"
			approved_any=1
		else
			log_warn "Skipping ${name} (SAN does not include ${ip}). SANs: $(echo "$san" | tr '\n' ' ')"
		fi
	done <<<"$csrs"

	if [[ $approved_any -eq 1 ]]; then
		log_info "Waiting for kubelet to pick up the new serving cert..."
		sleep 5
		microk8s status --wait-ready || true

		# Verify active kubelet cert SAN
		if sudo openssl x509 -in /var/snap/microk8s/current/certs/kubelet.crt -noout -ext subjectAltName |
			grep -q "IP Address:${ip}"; then
			log_info "✓ Kubelet serving cert now includes SAN ${ip}"
		else
			log_warn "Kubelet serving cert still missing SAN ${ip}. Check CSR list: microk8s kubectl get csr"
		fi
	else
		log_info "No CSR approved (none matched SAN ${ip})."
	fi
}

ensure_microk8s_networking() {
	log_section "Ensuring MicroK8s uses the correct interface"
	local ip
	ip="$(detect_node_ip)"
	if [[ -z $ip ]]; then
		log_error "Unable to detect a suitable node IP. Set K8S_NODE_IP=<ip> and retry."
		exit 1
	fi
	log_info "Detected node IP: ${ip}"

	# Optional safety: disable host-access (can create lo:microk8s -> 10.0.1.1)
	microk8s disable host-access >/dev/null 2>&1 || true

	# Apply launch configuration to pin kubelet and apiserver addresses.
	# Works on existing nodes via `snap set`.
	local cfg
	cfg=$(
		cat <<EOF
version: 0.1.0
extraKubeletArgs:
  --node-ip: ${ip}
  --rotate-server-certificates: "true"
extraKubeAPIServerArgs:
  --advertise-address: ${ip}
  --kubelet-preferred-address-types: InternalIP,Hostname,InternalDNS,ExternalDNS,ExternalIP
EOF
	)
	if command -v sudo >/dev/null 2>&1; then SUDO="sudo"; else SUDO=""; fi
	$SUDO snap set microk8s config="$cfg"
	$SUDO snap restart microk8s
	microk8s status --wait-ready

	# Show where the apiserver thinks kubelet lives now
	microk8s kubectl get nodes -o wide
	log_info "Pinned kubelet --node-ip and apiserver --advertise-address to ${ip}"

	$SUDO openssl x509 -in /var/snap/microk8s/current/certs/kubelet.crt -text -noout || true
}

check_prerequisites() {
	log_section "Checking Prerequisites"

	# Check MicroK8s
	if ! command -v microk8s &>/dev/null; then
		log_error "MicroK8s is not installed"
		exit 1
	fi

	# Check if MicroK8s is running (check output, not return code)
	local microk8s_status=$(microk8s status --wait-ready --format short --timeout 30 2>&1 || true)
	if echo "$microk8s_status" | grep -qi "not running\|stopped"; then
		log_error "MicroK8s is not running"
		log_info "Start with: microk8s start"
		exit 1
	fi

	log_info "✓ MicroK8s is running"

	# Ensure kubelet/apiserver use the expected host IP <<<
	K8S_NODE_IP=$(detect_node_ip)
	local ip="$K8S_NODE_IP"

	ensure_microk8s_networking
	approve_kubelet_serving_csr "$ip"

	# Check required addons
	# local required_addons=("dns" "storage" "community")
	# for addon in "${required_addons[@]}"; do
	#   if ! microk8s status | grep -q "$addon: enabled"; then
	#     log_warn "Enabling addon: $addon"
	#     microk8s enable "$addon"
	#     sleep 5
	#   else
	#     log_info "✓ Addon enabled: $addon"
	#   fi
	# done
	#

	# Set kubectl context
	microk8s kubectl config use-context microk8s || echo "Failed to set context to microk8s"
	log_info "✓ Using context: $(microk8s kubectl config current-context)"

	# Check if image exists
	if ! docker image ls "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}-dev" | grep -q "$APP_VERSION"; then
		log_warn "Image not found, building with --no-cache..."

		docker build \
			--tag "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}-dev" \
			--no-cache \
			--build-arg POLL_TIME="$POLL_TIME" \
			--build-arg PROFILES_DIR=/app/profiles \
			--label "build.time=$BUILD_TIME" \
			--label "git.commit=$GIT_SHA" \
			-f Dockerfile \
			.

		log_info "✓ Image built successfully"
	else
		log_info "✓ Image exists: ghcr.io/tuxerrante/kapparmor:${APP_VERSION}-dev"
	fi

	# Push to registry if needed
	if [ -n "${GH_WRITE_PKG_TOKEN:-}" ]; then
		log_info "Pushing image to registry..."
		echo "$GH_WRITE_PKG_TOKEN" | docker login -u "$(git config user.email)" --password-stdin ghcr.io
		docker push "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}-dev"
		log_info "✓ Image pushed"
	else
		log_warn "GH_WRITE_PKG_TOKEN not set, skipping registry push"
	fi
}

# ==========================================
# Deploy Kapparmor
# ==========================================

deploy_kapparmor() {
	log_section "Deploying Kapparmor"

	# Dry run first
	log_info "Running helm dry-run..."
	helm upgrade kapparmor --install \
		--atomic \
		--create-namespace \
		--debug \
		--devel \
		--dry-run \
		--namespace "$TARGET_NS" \
		--set image.pullPolicy=Always --set image.tag="${APP_VERSION}-dev" \
		--set "podAnnotations.gitCommit=$GIT_SHA" \
		--set "podAnnotations.build-time=$BUILD_TIME" \
		--timeout 120s \
		--wait \
		charts/kapparmor | tee -a "$LOG_FILE"

	echo ""
	log_info "Current K8S context: $(microk8s kubectl config current-context)"
	read -r -p "Continue with deployment? [Y/n] " response
	if [[ ! $response =~ ^([yY][eE][sS]|[yY]|)$ ]]; then
		log_warn "Deployment cancelled by user"
		exit 0
	fi

	# Real deployment
	log_info "Deploying Kapparmor..."
	helm upgrade kapparmor --install \
		--cleanup-on-fail \
		--create-namespace \
		--devel \
		--namespace "$TARGET_NS" \
		--set image.pullPolicy=Always --set image.tag="${APP_VERSION}-dev" \
		--set "podAnnotations.gitCommit=$GIT_SHA" \
		--set "podAnnotations.build-time=$BUILD_TIME" \
		--timeout 120s \
		--wait \
		charts/kapparmor

	log_info "Waiting for rollout to complete..."
	microk8s kubectl rollout status daemonset/kapparmor -n "$TARGET_NS" --timeout=120s

	log_info "✓ Kapparmor deployed successfully"

	# Show pod status
	echo "" | tee -a "$LOG_FILE"
	log_info "Pod Status:" | tee -a "$LOG_FILE"
	microk8s kubectl get pods -n "$TARGET_NS" -l=app.kubernetes.io/name=kapparmor -o wide | tee -a "$LOG_FILE"
}

# ==========================================
# Helper Functions
# ==========================================

show_logs() {
	local namespace="${1:-$TARGET_NS}"
	local lines="${2:-30}"

	echo "" | tee -a "$LOG_FILE"
	log_info "Recent logs from kapparmor pods:" | tee -a "$LOG_FILE"

	# Non far fallire la pipeline in modalità -euo pipefail
	set +e
	microk8s kubectl logs -n "$namespace" \
		-l=app.kubernetes.io/name=kapparmor \
		--tail="$lines" --prefix=true |
		tee -a "$LOG_FILE" || true
	set -e
}

show_events() {
	local namespace="${1:-$TARGET_NS}"

	echo "" | tee -a "$LOG_FILE"
	log_info "Recent events in namespace $namespace:" | tee -a "$LOG_FILE"

	set +e
	microk8s kubectl get events -n "$namespace" --sort-by='.lastTimestamp' |
		tail -20 | tee -a "$LOG_FILE" || true
	set -e
}

# Read recent logs, extract *the last* line that contains msg="retrieving profiles",
# and check if that line contains the expected substring.
wait_for_last_retrieving_profiles_contains() {
	local expected="$1"
	local max_wait="${2:-60}"
	local tail_n="${3:-200}" # how many recent lines to scan

	log_info "Waiting for the *last* 'retrieving profiles' line to contain: '$expected' (max ${max_wait}s)..."
	local elapsed=0

	while [ "$elapsed" -lt "$max_wait" ]; do
		# Pull recent logs from all kapparmor pods and keep only the last matching line
		local last_line
		last_line="$(
			microk8s kubectl logs -n "$TARGET_NS" -l=app.kubernetes.io/name=kapparmor --tail="$tail_n" --prefix=true 2>/dev/null |
				awk '/msg="retrieving profiles"/ { last=$0 } END { if (last) print last }'
		)"

		if [ -n "$last_line" ]; then
			log_info "Last 'retrieving profiles' line: $last_line"
			if printf '%s' "$last_line" | grep -F -q -- "$expected"; then
				log_info "✓ Last 'retrieving profiles' line contains '$expected'"
				return 0
			fi
		fi

		sleep 3
		elapsed=$((elapsed + 3))
	done

	log_warn "Timed out: last 'retrieving profiles' line did not contain '$expected' after ${max_wait}s"
	return 1
}

wait_for_profile_sync() {
	local profile_name="$1"
	local max_wait="${2:-60}"

	log_info "Waiting for profile '$profile_name' to sync (max ${max_wait}s)..."

	local elapsed=0
	while [ $elapsed -lt $max_wait ]; do
		if microk8s kubectl logs -n "$TARGET_NS" -l=app.kubernetes.io/name=kapparmor --tail=20 |
			grep -F -q -- "$profile_name"; then
			log_info "✓ Profile '$profile_name' detected in logs"
			return 0
		fi
		sleep 5
		elapsed=$((elapsed + 5))
	done

	log_warn "Profile '$profile_name' not detected in logs after ${max_wait}s"
	return 1
}

# Print last N lines from all kapparmor DS pods
show_kapparmor_ds_logs() {
	local tail="${1:-80}"

	echo "" | tee -a "$LOG_FILE"
	log_info "Kapparmor DaemonSet logs (last ${tail} lines per pod):"

	set +e
	microk8s kubectl logs \
		-n "$TARGET_NS" \
		-l app.kubernetes.io/name=kapparmor \
		--tail="$tail" --prefix=true |
		tee -a "$LOG_FILE" || true
	set -e
}

# Apply kapparmor-profiles ConfigMap from file,
# always print the live object after apply,
# and optionally force .data={} to guarantee empty keys.
# Usage:
#   apply_profiles_cm <file> [ensure_empty]
apply_profiles_cm() {
	local file="$1"
	local ensure_empty="${2:-}"

	if [[ -z $file || ! -f $file ]]; then
		log_error "ConfigMap file not found: ${file:-<empty>}"
		return 1
	fi

	log_info "Applying ConfigMap from: $file (server-side apply)"
	# Server-Side Apply avoids the 'last-applied-configuration' warning and lets us own fields.
	# --force-conflicts takes ownership from Helm when needed.
	if ! microk8s kubectl apply -n "$TARGET_NS" --server-side --force-conflicts -f "$file" | tee -a "$LOG_FILE"; then
		log_error "kubectl apply failed for $file"
		return 1
	fi

	# Optionally *guarantee* empty data (removes all keys regardless of previous owner)
	if [[ $ensure_empty == "ensure_empty" ]]; then
		log_info "Ensuring ConfigMap .data is empty via patch"
		microk8s kubectl patch configmap kapparmor-profiles \
			-n "$TARGET_NS" --type=merge -p '{"data":{}}' |
			tee -a "$LOG_FILE" || true
	fi

	# Print the live ConfigMap (full YAML) so we can see exactly what's mounted
	log_info "Current kapparmor-profiles (live object):"
	microk8s kubectl get configmap kapparmor-profiles -n "$TARGET_NS" -o yaml |
		tee -a "$LOG_FILE" || true
}

# ==========================================
# TEST CASE 1
# ==========================================
test_case_1_profile_management() {
	log_section "TEST CASE 1: Profile Management"

	local CM_EMPTY_FILE="test/cm-kapparmor-empty.yml"
	local CM_ONE_FILE="test/cm-kapparmor-home-profile.yml"
	local CM_ONE_FILE_EDITED="test/cm-kapparmor-home-profile-edited.yml"
	local EXPECTED_PROFILE="custom.deny-write-outside-home"

	# Step 0: Snapshot initial DS logs
	log_info "Initial Kapparmor DaemonSet logs:"
	show_kapparmor_ds_logs 60

	# --------------------------
	# Step 1: Apply EMPTY ConfigMap
	# --------------------------
	log_test "---> Apply EMPTY ConfigMap"
	if apply_profiles_cm "$CM_EMPTY_FILE" ensure_empty; then
		test_passed "Empty ConfigMap applied & printed"
	else
		test_failed "Empty ConfigMap apply"
		# Keep going; do not abort whole suite
	fi

	log_info "Waiting ${DELAY_SECONDS}s for sync (POLL_TIME + buffer)"
	sleep "$DELAY_SECONDS"

	log_info "DaemonSet logs after empty ConfigMap:"
	show_kapparmor_ds_logs 80

	# Validate ABSENCE of the profile in logs
	set +e
	if wait_for_last_retrieving_profiles_contains "profiles=[]" 20; then
		test_passed "String 'profiles=[]' found after applying an empty ConfigMap"
	else
		test_failed "String 'profiles=[]' absent after empty ConfigMap"
	fi
	set -e

	# --------------------------
	# Step 2: Apply ONE-PROFILE ConfigMap
	# --------------------------
	log_test "Apply ONE-PROFILE ConfigMap"
	if apply_profiles_cm "$CM_ONE_FILE"; then
		test_passed "One-profile ConfigMap applied & printed"
	else
		test_failed "One-profile ConfigMap apply"
		# Continue anyway to surface subsequent steps/logs
	fi

	log_info "Waiting ${DELAY_SECONDS}s for sync (POLL_TIME + buffer)"
	sleep "$DELAY_SECONDS"

	log_info "DaemonSet logs after one-profile ConfigMap:"
	show_kapparmor_ds_logs 120

	# Validate PRESENCE
	set +e
	if wait_for_profile_sync "$EXPECTED_PROFILE" 45; then
		test_passed "Profile '$EXPECTED_PROFILE' synced successfully"
	else
		test_failed "Profile '$EXPECTED_PROFILE' NOT found after applying ConfigMap"
	fi
	set -e

	# --------------------------
	# Step 3: Apply EDITED profile (content change)
	# --------------------------
	log_test "Apply EDITED one-profile ConfigMap"
	if apply_profiles_cm "$CM_ONE_FILE_EDITED"; then
		test_passed "Edited profile ConfigMap applied & printed"
	else
		test_failed "Edit-profile ConfigMap apply"
	fi

	log_info "Waiting ${DELAY_SECONDS}s for sync (POLL_TIME + buffer)"
	sleep "$DELAY_SECONDS"

	log_info "DaemonSet logs after CM edit:"
	show_kapparmor_ds_logs 120

	# Validate PRESENCE again (name unchanged; content expected to update)
	set +e
	if wait_for_profile_sync "$EXPECTED_PROFILE" 45; then
		test_passed "Profile '$EXPECTED_PROFILE' still detected after CM edit"
	else
		test_failed "Profile '$EXPECTED_PROFILE' NOT detected after CM edit"
	fi
	set -e

	log_info "Test Case 1 completed."
}

# ==========================================
# TEST CASE 2: Profile Deletion While In Use
# ==========================================

test_case_2_profile_in_use() {
	log_section "TEST CASE 2: Profile Deletion While In Use"

	# Ensure test namespace exists
	microk8s kubectl create namespace "$TEST_NS" || true

	# Step 1: Add profile for home directory restriction
	log_test "Step 1: Add custom.deny-write-outside-home profile"

	# First, get existing data
	local existing_data=$(microk8s kubectl get configmap kapparmor-profiles -n "$TARGET_NS" -o json 2>/dev/null | jq -r '.data // {}')

	# Add new profile to existing data
	cat <<EOF | microk8s kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: kapparmor-profiles
  namespace: $TARGET_NS
data:
  custom.deny-write-outside-home: |
    #include <tunables/global>
    
    profile custom.deny-write-outside-home flags=(attach_disconnected,mediate_deleted) {
      #include <abstractions/base>
      
      file,
      network,
      capability,
      
      # Allow writes only in /home
      /home/** rw,
      deny /** w,
    }
EOF

	log_info "Waiting ${DELAY_SECONDS}s for profile to sync..."
	sleep "$DELAY_SECONDS"

	if wait_for_profile_sync "custom.deny-write-outside-home" 30; then
		test_passed "Profile added and synced"
	else
		test_failed "Profile not synced"
		return 1
	fi

	# Step 2: Deploy pod using the profile
	log_test "Step 2: Deploy ubuntu pod with profile"

	cat <<EOF | microk8s kubectl apply -n "$TEST_NS" -f -
apiVersion: v1
kind: Pod
metadata:
  name: ubuntu-custom-profile
spec:
  securityContext:
    appArmorProfile:
        type: Localhost
        localhostProfile: custom.deny-write-outside-home
  containers:
  - name: ubuntu
    image: ubuntu
    command: [ "sh", "-c", "echo 'Hello AppArmor!' && sleep 1h" ]
    resources:
      limits:
        cpu: 100m
        memory: 128Mi
    securityContext:
      runAsUser: 0
  restartPolicy: Always
EOF

	log_info "Waiting for pod to be ready..."
	if microk8s kubectl wait --for=condition=Ready pod/ubuntu-custom-profile -n "$TEST_NS" --timeout=60s; then
		test_passed "Pod deployed successfully"
	else
		log_error "Pod failed to start"
		microk8s kubectl describe pod ubuntu-custom-profile -n "$TEST_NS" | tee -a "$LOG_FILE"
		test_failed "Pod deployment"
		return 1
	fi

	# Step 3: Verify profile is active
	log_test "Step 3: Verify profile on running pod"

	local current_profile=$(microk8s kubectl exec ubuntu-custom-profile -n "$TEST_NS" -- cat /proc/1/attr/current 2>/dev/null || echo "failed")
	log_info "Current AppArmor profile: $current_profile"

	if echo "$current_profile" | grep -q "custom.deny-write-outside-home"; then
		test_passed "Profile active on pod"
	else
		log_warn "Profile not active: $current_profile"
	fi

	# Step 4: Attempt to delete profile while pod is using it
	log_test "Step 4: Attempt to delete profile while pod is running"

	log_info "Trying to remove profile from ConfigMap..."
	if microk8s kubectl patch configmap kapparmor-profiles -n "$TARGET_NS" \
		--type='json' \
		-p='[{"op": "remove", "path": "/data/custom.deny-write-outside-home"}]' 2>&1 | tee -a "$LOG_FILE"; then
		log_info "ConfigMap updated (profile removed from config)"
		test_passed "Profile removed from ConfigMap"
	else
		log_error "Failed to update ConfigMap"
		test_failed "ConfigMap update"
	fi

	log_info "Waiting ${DELAY_SECONDS}s for changes to propagate..."
	sleep "$DELAY_SECONDS"

	# Step 5: Check if pod is still running
	log_test "Step 5: Verify pod still running"

	if microk8s kubectl get pod ubuntu-custom-profile -n "$TEST_NS" -o jsonpath='{.status.phase}' | grep -q "Running"; then
		test_passed "Pod still running after profile removed from ConfigMap"
		log_info "This is expected - running pods keep their profile until restart"
	else
		log_warn "Pod not running (unexpected)"
		microk8s kubectl describe pod ubuntu-custom-profile -n "$TEST_NS" | tee -a "$LOG_FILE"
	fi

	# Step 6: Check profile status on running pod
	log_test "Step 6: Check if profile still active on running pod"

	local profile_after=$(microk8s kubectl exec ubuntu-custom-profile -n "$TEST_NS" -- cat /proc/1/attr/current 2>/dev/null || echo "failed")
	log_info "Profile after ConfigMap update: $profile_after"

	if echo "$profile_after" | grep -q "custom.deny-write-outside-home"; then
		test_passed "Profile still active on running pod (correct behavior)"
		log_info "Profile persists on running container until pod restart"
	else
		log_warn "Profile changed/removed from running pod: $profile_after"
	fi

	# Step 7: Check kapparmor logs for warnings
	log_test "Step 7: Check kapparmor logs for any issues"

	show_logs "$TARGET_NS" 30

	if microk8s kubectl logs -n "$TARGET_NS" -l=app.kubernetes.io/name=kapparmor --tail=50 | grep -qi "error\|failed"; then
		log_warn "Found errors in kapparmor logs (check above)"
	else
		log_info "No errors in kapparmor logs"
	fi

	# Step 8: Test if new pods can still start
	log_test "Step 8: Try to create new pod with removed profile"
	local pod8_name="test8-$(date -u +"%Y%m%d%H%M")"
	cat <<EOF | microk8s kubectl apply -n "$TEST_NS" -f - 2>&1 | tee -a "$LOG_FILE" || true
apiVersion: v1
kind: Pod
metadata:
  name: $pod8_name
spec:
  securityContext:
    appArmorProfile:
        type: Localhost
        localhostProfile: custom.deny-write-outside-home
  containers:
  - name: ubuntu
    image: ubuntu
    command: [ "sh", "-c", "sleep 600" ]
    resources:
      limits:
        cpu: 100m
        memory: 128Mi
  restartPolicy: Never
EOF

	# Wait for the container status block to exist, or time out
	log_info "Waiting for pod $pod8_name container status to be populated..."
  microk8s kubectl wait pod/$pod8_name -n "$TEST_NS" \
		--for=jsonpath='{.status.containerStatuses[0].state.waiting.reason}' \
		--timeout=30s 2>&1 | tee -a "$LOG_FILE" || true

	local pod8_status=$(microk8s kubectl get pod $pod8_name -n "$TEST_NS" \
		-o jsonpath='{.status.phase}' 2>/dev/null || echo "NotFound")
  microk8s kubectl get pod $pod8_name -n "$TEST_NS" -o yaml | tee -a "$LOG_FILE" || true 
	# local container_status=$(microk8s kubectl get pod $pod8_name -n "$TEST_NS" \
	# -o jsonpath='{.status.containerStatuses[0].state.waiting.reason}' 2>/dev/null || echo "")
	# Get the reason from EITHER the waiting or terminated state
	sleep 10
	local container_status=$(microk8s kubectl get pod $pod8_name -n "$TEST_NS" \
		-o jsonpath='{.status.containerStatuses[0].state.waiting.reason}{.status.containerStatuses[0].state.terminated.reason}' 2>/dev/null || echo "")
	log_info "New pod status: $pod8_status, container status: $container_status"

	if [[ $pod8_status == "Pending" && $container_status == "CreateContainerError" ]] || [[ $pod8_status == "Failed" ]]; then
		test_passed "New pod blocked (profile no longer available)"
		log_info "Expected: Kubernetes should block pods requiring missing profiles"
	else
		log_warn "New pod started despite profile being removed (unexpected: status=$pod8_status, container=$container_status)"
		test_failed "Profile enforcement"
	fi

	# Cleanup
	microk8s kubectl delete pod ubuntu-custom-profile -n "$TEST_NS" --wait=false || true
	microk8s kubectl delete pod $pod8_name -n "$TEST_NS" --wait=false || true
}

# ==========================================
# Final Status Check
# ==========================================
final_status_check() {
	log_section "Final Status Check"

	# Evita abort su comandi informativi sotto -euo pipefail
	set +e

	log_info "Kapparmor Pod Status:" | tee -a "$LOG_FILE"
	microk8s kubectl get pods -n "$TARGET_NS" \
		-l=app.kubernetes.io/name=kapparmor -o wide |
		tee -a "$LOG_FILE" || true
	echo "" | tee -a "$LOG_FILE"

	log_info "Kapparmor DaemonSet Status:" | tee -a "$LOG_FILE"
	microk8s kubectl get daemonset kapparmor -n "$TARGET_NS" |
		tee -a "$LOG_FILE" || true
	echo "" | tee -a "$LOG_FILE"

	log_info "Current ConfigMap Profiles:" | tee -a "$LOG_FILE"
	# Stampa solo da 'data:' in giù se esiste; altrimenti messaggio esplicito.
	local cm_yaml cm_data
	cm_yaml="$(microk8s kubectl get configmap kapparmor-profiles -n "$TARGET_NS" -o yaml 2>/dev/null || true)"
	if [ -n "$cm_yaml" ]; then
		cm_data="$(printf '%s\n' "$cm_yaml" | sed -n '/^data:/,$p')"
		if [ -n "$cm_data" ]; then
			printf '%s\n' "$cm_data" | tee -a "$LOG_FILE" >/dev/null
		else
			echo "(no 'data' keys present)" | tee -a "$LOG_FILE"
		fi
	else
		echo "(configmap kapparmor-profiles not found)" | tee -a "$LOG_FILE"
	fi
	echo "" | tee -a "$LOG_FILE"

	# Log & events (non bloccano il flusso)
	show_logs "$TARGET_NS" 40 || true
	show_events "$TARGET_NS" || true

	set -e
}

# ==========================================
# Test Summary
# ==========================================
print_summary() {
	log_section "Test Summary"
	echo "" | tee -a "$LOG_FILE"

	local total=$((TESTS_PASSED + TESTS_FAILED))
	printf "Total: %d | Passed: %d | Failed: %d\n" \
		"$total" "$TESTS_PASSED" "$TESTS_FAILED" |
		tee -a "$LOG_FILE"

	echo "" | tee -a "$LOG_FILE"

	if [ "$TESTS_FAILED" -eq 0 ]; then
		log_info "✓ All tests passed!"
		log_info "Log file: $LOG_FILE"
		return 0
	else
		log_error "✗ Some tests failed"
		echo "Hint: grep -E '^(\\[FAIL\\]|\\[WARN\\])' '$LOG_FILE' -n" | tee -a "$LOG_FILE"
		log_error "Check log file: $LOG_FILE"
		return 1
	fi
}

# ==========================================
# Main Execution
# ==========================================

main() {
	log_section "Kapparmor MicroK8s Integration Tests"
	log_info "Start time: $(date)"
	log_info "Git commit: $GIT_SHA"
	log_info "Build time: $BUILD_TIME"
	log_info "Log file: $LOG_FILE"

	check_prerequisites
	deploy_kapparmor

	echo ""
	log_info "Waiting ${DELAY_SECONDS}s for initial sync..."
	sleep "$DELAY_SECONDS"

	show_logs "$TARGET_NS" 20

	# Run test cases
	test_case_1_profile_management
	echo ""
	test_case_2_profile_in_use

	# Final checks
	final_status_check

	# Summary
	print_summary
}

# Run main
main "$@"
