#!/bin/bash
set -e

. ./build/build-app.sh
. ./config/secrets

echo $GHCR_TOKEN | docker login -u "$(git config user.email)" --password-stdin ghcr.io
docker push ghcr.io/tuxerrante/kapparmor:${APP_VERSION}_dev

# Install the chart from the local directory
helm upgrade kapparmor \
    --install \
    --atomic \
    --debug \
    --set app.poll_time=30 \
    --set image.tag=${APP_VERSION}_dev \
    --set image.pullPolicy=Always \
    --dry-run \
    charts/kapparmor

echo
echo "> Is the previous result the expected one?"
echo "> Current K8S context:" "$(kubectl config current-context)"
read -r -p "> Are you sure? [Y/n] " response
if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
    helm upgrade kapparmor --install \
        --atomic \
        --timeout 120s \
        --debug \
        --set app.poll_time=30 \
        --set image.tag=${APP_VERSION}_dev \
        --set image.pullPolicy=Always \
        charts/kapparmor
else
    echo " Bye."
    echo
fi

export POD_NAME=$(kubectl get pods --namespace default -l "app.kubernetes.io/name=kapparmor,app.kubernetes.io/instance=kapparmor" -o jsonpath="{.items[0].metadata.name}")
kubectl wait --for=jsonpath='{.status.numberReady}'=1 daemonset/kapparmor
echo
kubectl get pods -l=app.kubernetes.io/name=kapparmor -o wide
echo
kubectl logs --follow -l=app.kubernetes.io/name=kapparmor
