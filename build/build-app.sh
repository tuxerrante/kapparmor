#!/bin/bash
source ./config/config

# --- Validate App and Chart version
YML_CHART_VERSION="$(grep "version: [\"0-9\.]\+" charts/kapparmor/Chart.yaml  |cut -d'"' -f2)"
YML_APP_VERSION="$(grep "appVersion: [\"0-9\.]\+" charts/kapparmor/Chart.yaml |cut -d'"' -f2)"

if [[ $APP_VERSION != $YML_APP_VERSION ]]; then
    echo "The APP version declared in the Chart is different from the one in the config!"
    exit 1
elif [[ $CHART_VERSION != $YML_CHART_VERSION ]]; then
    echo "The APP version declared in the Chart is different from the one in the config!"
    exit 1
fi

# Clean old images
echo "> Removing old and dangling old images..."
docker rmi "$(docker images --filter "reference=ghcr.io/tuxerrante/kapparmor" -q --no-trunc )"

# go build -o ./.go/bin ./...
# go test -v -coverprofile=coverage.out -covermode=atomic ./go/src/app/...
if [[ ! -f ".go/bin/golangci-lint" ]]; then
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./.go/bin
fi
echo "> Linting..."
./.go/bin/golangci-lint run

echo "> Scanning for suspicious constructs..."
go vet go/...

echo "> Creating test output..."
docker build --target test-coverage --tag "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}_dev" .

#### To run it look into docs/testing.md
echo "> Building container image..."
docker build --tag "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}_dev" \
    --no-cache	  \
    --build-arg POLL_TIME=30 \
    --build-arg PROFILES_DIR=/app/profiles   \
    -f Dockerfile \
    .

# docker run --rm -it --privileged --mount type=bind,source='/sys/kernel/security',target='/sys/kernel/security' --mount type=bind,source='/etc',target='/etc' --name kapparmor  ghcr.io/tuxerrante/kapparmor:${APP_VERSION}_dev
