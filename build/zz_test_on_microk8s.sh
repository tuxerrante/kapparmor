#!/bin/bash
set -e

#  --- Configuration ---
# . ./build/test_on_docker.sh
. ./config/config
. ./config/secrets

DELAY_SECONDS=$((POLL_TIME + 10))
GIT_SHA=$(git rev-parse --short=12 HEAD)
TARGET_NS="default"
kubectl config use-context microk8s
# ---

docker image ls ghcr.io/tuxerrante/kapparmor

echo $GH_TOKEN | docker login -u "$(git config user.email)" --password-stdin ghcr.io
docker push ghcr.io/tuxerrante/kapparmor:${APP_VERSION}-dev

# Test the chart from the local directory
helm upgrade kapparmor --install \
  --atomic \
  --create-namespace \
  --debug \
  --devel \
  --dry-run \
  --namespace ${TARGET_NS} \
  --set image.pullPolicy=Always \
  --set image.tag=${APP_VERSION}-dev \
  --set podAnnotations.gitCommit="$GIT_SHA" \
  --timeout 120s \
  --wait \
  charts/kapparmor

echo
echo "> Current K8S context:" "$(kubectl config current-context)"
echo "> Is the previous result the expected one?"
read -r -p "> Are you sure? [Y/n] " response
if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
  helm upgrade kapparmor --install \
    --cleanup-on-fail \
    --create-namespace \
    --debug \
    --devel \
    --namespace ${TARGET_NS} \
    --set image.pullPolicy=IfNotPresent \
    --set image.tag=${APP_VERSION}-dev \
    --set podAnnotations.gitCommit="$GIT_SHA" \
    --timeout 120s \
    --wait \
    charts/kapparmor
else
  echo " Bye."
  echo
fi

echo
kubectl get pods -l=app.kubernetes.io/name=kapparmor -o wide -w

echo "> Waiting ${DELAY_SECONDS} seconds for changes to propagate..."
sleep ${DELAY_SECONDS}
echo "> Check logs"
kubectl -n ${TARGET_NS} logs -l=app.kubernetes.io/name=kapparmor --tail=20

echo "> Test: empty the kapparmor configmap to see if profiles are removed from nodes"
kubectl -n ${TARGET_NS} patch configmap kapparmor-profiles \
  --type='json' \
  -p='[{"op": "replace", "path": "/data", "value": {}}]'

echo "> Waiting ${DELAY_SECONDS} seconds for changes to propagate..."
sleep ${DELAY_SECONDS}
echo "> Check logs"
kubectl -n ${TARGET_NS} logs -l=app.kubernetes.io/name=kapparmor --tail=20
