#!/bin/bash
set -euo pipefail

#  --- Configuration ---
source ./config/config
CONTAINER_NAME="kapparmor"
APP_VERSION="${APP_VERSION:-latest}"
HOST_FILE_PATH="charts/kapparmor/profiles/custom.deny-write-outside-app"
CONTAINER_TARGET_PATH="/app/profiles/custom.deny-write-outside-app"
DELAY_SECONDS=$(( POLL_TIME + 10 ))
# POLL_TIME=10
START_TIMEDATE=$(date +%Y%m%d_%H%M%S)
LOG_FILE=".kapparmor_test_${START_TIMEDATE}.txt"
START_TIME=$(date +%H:%M)
echo "> Logging to ${LOG_FILE}"
# ---

# Take care of existing Kapparmor containers running in a local cluster
echo "> Checking for existing Kapparmor containers in local Kubernetes clusters..." | tee -a "${LOG_FILE}"
kubectl config use-context microk8s
MICROK8S_STATUS=$(microk8s status --format short)
if [[ "${MICROK8S_STATUS}" == *"is running"* ]]; then
  echo "> MicroK8s is running. Checking for Kapparmor pods..." | tee -a "${LOG_FILE}"

  # Find DaemonSets named kapparmor
  KAPP_DAEMONSET=$(microk8s kubectl get daemonset --all-namespaces \
    -o jsonpath="{.items[?(@.metadata.name=='kapparmor')].metadata.name}")

  # Find Helm releases (Helm-managed resources usually have label 'app.kubernetes.io/instance')
  helm list --all-namespaces --filter '^kapparmor$' --short || echo "No Helm releases found." | tee -a "${LOG_FILE}"

  if [[ -n "${KAPP_DAEMONSET}" ]]; then
    echo "> Found Kapparmor pods in MicroK8s" | tee -a "${LOG_FILE}"
    echo "> Stopping MicroK8s to avoid conflicts..." | tee -a "${LOG_FILE}"
    microk8s stop | tee -a "${LOG_FILE}"
  else
    echo "> No Kapparmor pods found in MicroK8s." | tee -a "${LOG_FILE}"
  fi
else
  echo "> MicroK8s is not running." | tee -a "${LOG_FILE}"
fi

sudo apparmor_status --show=profiles --filter.profiles=custom* | tee -a "${LOG_FILE}"
for profile in ./charts/kapparmor/profiles/custom*; do
  profile_name=$(basename "$profile")
  echo "> Unloading profile: $profile_name" | tee -a "${LOG_FILE}"
  sudo apparmor_parser --verbose --remove "./charts/kapparmor/profiles/${profile_name}" ||
    echo "Profile $profile_name not loaded, skipping removal." | tee -a "${LOG_FILE}"
done

echo "> Removing profiles files from /etc/apparmor.d/custom/" | tee -a "${LOG_FILE}"
sudo mkdir -p /etc/apparmor.d/custom/
sudo rm --verbose -f /etc/apparmor.d/custom/custom*
sudo service apparmor reload || sudo systemctl reload apparmor | tee -a "${LOG_FILE}"

# --- Validate App and Chart version
YML_CHART_VERSION="$(grep "version: [\"0-9\.]\+" charts/kapparmor/Chart.yaml | cut -d'"' -f2)"
YML_APP_VERSION="$(grep "appVersion: [\"0-9\.]\+" charts/kapparmor/Chart.yaml | cut -d'"' -f2)"

if [[ $APP_VERSION != "${YML_APP_VERSION}" ]]; then
  echo "> config/config/APP_VERSION = |${APP_VERSION}|"
  echo "> charts/kapparmor/Chart.yaml/YML_APP_VERSION = |${YML_APP_VERSION}|"
  echo "The APP version declared in the Chart is different from the one in the config!"
  exit 1
elif [[ $CHART_VERSION != "$YML_CHART_VERSION" ]]; then
  echo "The CHART version declared in the Chart is different from the one in the config!"
  exit 1
fi

# Set same Go version in all relevant files
echo "Set go version variables in .github/workflows to $GO_VERSION"
find .github/workflows -type f -exec sed -i 's/go-version: .*/go-version: '"$GO_VERSION"'/g' {} +
find .github/workflows -type f -exec sed -i 's/GO_VERSION: .*/GO_VERSION: '"$GO_VERSION"'/g' {} +
sed -i 's/^go [0-9]\(\.[0-9]\+\)/go '"$GO_VERSION"'/g' go.mod
sed -i 's/golang:[0-9]\(\.[0-9]\+\)/golang:'"$GO_VERSION"'/g' Dockerfile

# Clean old images
echo "> Removing old and dangling old images..."
docker rmi "$(docker images --filter "reference=ghcr.io/tuxerrante/kapparmor" -q --no-trunc)" || echo "No old images to remove."

# Testing and linting moved to Makefile and pre-commit hooks
# go test -v -coverprofile=coverage.out -covermode=atomic ./src/app/...
# if [[ ! -f ".go/bin/golangci-lint" ]]; then
#     curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./.go/bin
# fi
# echo "> Linting..."
# ./.go/bin/golangci-lint run
#
# echo "> Scanning for suspicious constructs..."
# go vet go/...
#
# echo "> Creating test output..."
# docker build --target test-coverage --tag "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}-dev" -f Dockerfile .

#### To run it look into docs/testing.md
echo "> Building container image..."
docker build --tag "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}-dev" \
  --build-arg POLL_TIME=$POLL_TIME \
  --build-arg PROFILES_DIR=/app/profiles \
  -f Dockerfile \
  .

echo
grype --fail-on critical "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}-dev"

echo "> Audit profile changes"
# sudo inotifywait --monitor --timeout 90 -e modify,attrib,create,delete /etc/apparmor.d/custom/ &
# sudo auditctl -w /etc/apparmor.d/custom/ -p wa -k kapparmor-watch | tee -a "${LOG_FILE}"
sudo auditctl -a always,exit -F arch=b64 -F dir=/etc/apparmor.d/custom/ -F perm=wa -k kapparmor-watch || echo "> audictl kapparmor-watch rule already exists."

echo "> Starting container ${CONTAINER_NAME} in detached mode..." | tee -a "${LOG_FILE}"
CONTAINER_ID=$(docker run -d --rm --privileged --init \
  --mount type=bind,source='/sys/kernel/security',target='/sys/kernel/security' \
  --mount type=bind,source='/etc',target='/etc' \
  --name "${CONTAINER_NAME}" \
  "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}-dev")

echo "> Container started: ${CONTAINER_ID}" | tee -a "${LOG_FILE}"

# Stream logs asynchronously into file
# pstree -pa "$(pgrep -fa app | awk '{print $1}')" || true | tee -a "${LOG_FILE}"
docker logs -f "${CONTAINER_ID}" >>"${LOG_FILE}" 2>&1 &
LOG_PID=$!

echo "Waiting ${DELAY_SECONDS} seconds before injecting a profile into the container..."
sleep ${DELAY_SECONDS}

echo "> Injecting ${HOST_FILE_PATH} into container..." | tee -a "${LOG_FILE}"
if docker cp "${HOST_FILE_PATH}" "${CONTAINER_NAME}:${CONTAINER_TARGET_PATH}" >>"${LOG_FILE}" 2>&1; then
  echo "File injection successful." | tee -a "${LOG_FILE}"
else
  echo ">>ERROR: docker cp failed or container not running." | tee -a "${LOG_FILE}"
fi

sleep ${DELAY_SECONDS}
echo "> Stopping container..." | tee -a "${LOG_FILE}"
sudo docker stop --time=5 "${CONTAINER_NAME}" || docker kill $CONTAINER_NAME >>"${LOG_FILE}" 2>&1 || echo "Container already stopped" | tee -a "${LOG_FILE}"

wait ${LOG_PID} 2>/dev/null || true

echo "> Check for orphaned processes..." | tee -a "${LOG_FILE}"
pstree -pa "$(pgrep -fa app | awk '{print $1}')" || true | tee -a "${LOG_FILE}"

# --- Post-run logs ---
echo "> Collecting AppArmor and audit logs..." | tee -a "${LOG_FILE}"
sudo apparmor_status --show=profiles --filter.profiles=custom* | tee -a "${LOG_FILE}"
sudo ausearch -k kapparmor-watch --start ${START_TIME} | tee -a "${LOG_FILE}" || true

# --- Cleanup ---
echo "> Cleaning audit rules..." | tee -a "${LOG_FILE}"
# sudo auditctl -W /etc/apparmor.d/custom/ -p wa -k kapparmor-watch || true
sudo auditctl -d always,exit -F arch=b64 -F dir=/etc/apparmor.d/custom/ -F perm=wa -k kapparmor-watch
