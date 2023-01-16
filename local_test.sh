#!/bin/bash
# https://github.com/helm/chart-testing
# https://github.com/helm/chart-testing/issues/464
echo Linting the Helm chart

docker run -it --network host --workdir=/data --volume ~/.kube/config:/root/.kube/config:ro \
  --volume $(pwd):/data quay.io/helmpack/chart-testing:latest \
  /bin/sh -c "git config --global --add safe.directory /data; ct lint --print-config --charts ./charts/kapparmor"
