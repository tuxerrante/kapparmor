#!/bin/bash
set -e

. ./build/build-app.sh
. ./config/secrets

echo $GHCR_TOKEN | docker login -u "$(git config user.email)" --password-stdin ghcr.io
docker push ghcr.io/tuxerrante/kapparmor:${APP_VERSION}_dev

# Install the chart from the local directory
helm upgrade kapparmor --install \
    --atomic \
    --debug \
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
        --set image.tag=${APP_VERSION}_dev \
        --set image.pullPolicy=Always \
        charts/kapparmor
else
    echo " Bye."
    echo
fi
