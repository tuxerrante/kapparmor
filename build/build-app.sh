#!/bin/bash
source ./config/config

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
