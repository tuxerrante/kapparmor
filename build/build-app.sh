#!/bin/bash
source ./config/config

# --- Validate App and Chart version
YML_CHART_VERSION="$(grep "version: [\"0-9\.]\+" charts/kapparmor/Chart.yaml | cut -d'"' -f2)"
YML_APP_VERSION="$(grep "appVersion: [\"0-9\.]\+" charts/kapparmor/Chart.yaml | cut -d'"' -f2)"

if [[ $APP_VERSION != ${YML_APP_VERSION} ]]; then
    echo "> config/config/APP_VERSION = |${APP_VERSION}|"
    echo "> charts/kapparmor/Chart.yaml/YML_APP_VERSION = |${YML_APP_VERSION}|"
    echo "The APP version declared in the Chart is different from the one in the config!"
    exit 1
elif [[ $CHART_VERSION != $YML_CHART_VERSION ]]; then
    echo "The CHART version declared in the Chart is different from the one in the config!"
    exit 1
fi

# Set same Go version in all relevant files
echo "Set go version variables in .github/workflows to $GO_VERSION"
find .github/workflows -type f -exec sed -i 's/go-version: .*/go-version: '"$GO_VERSION"'/g' {} +
find .github/workflows -type f -exec sed -i 's/GO_VERSION: .*/GO_VERSION: '"$GO_VERSION"'/g' {} +
sed -i 's/^go [0-9\.].*/go '"$GO_VERSION"'/g' go.mod

dockerfile_go_version=$(grep -o 'golang:[0-9.]\+' Dockerfile)
if [[ $dockerfile_go_version != "golang:${GO_VERSION}" ]]; then
    echo "Dockerfile go version ($dockerfile_go_version) is different from the configuration ($GO_VERSION)".
    echo "Searching for golang:${GO_VERSION} with crane..."
    docker run --rm gcr.io/go-containerregistry/crane digest golang:1.22.0
    exit 1
fi

# Clean old images
echo "> Removing old and dangling old images..."
docker rmi "$(docker images --filter "reference=ghcr.io/tuxerrante/kapparmor" -q --no-trunc)" || true

# Clean go cache
# go clean ./...

# Lint and try fixing
if [[ ! -f "./bin/golangci-lint" ]]; then
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin
fi
echo "> Linting..."
./bin/golangci-lint run --go "$GO_VERSION" --fix --fast --allow-parallel-runners ./go/src/app

echo Linting the Helm chart
helm lint --debug --strict  charts/kapparmor/

# --- Unit tests are in Dockerfile
# go test -v -vet=off -failfast -coverprofile=coverage.out -covermode=atomic ./go/src/app/...
# echo "> Scanning for suspicious constructs..."
# go vet go/...
# echo "> Creating test output..."
# docker build --target test-coverage --tag "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}_dev" .

#### To run it look into docs/testing.md
echo "> Building container image..."
docker build --tag "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}_dev" \
    --no-cache \
    --build-arg POLL_TIME=30 \
    --build-arg PROFILES_DIR=/app/profiles \
    -f Dockerfile \
    .

if [[ ! -f "./bin/trivy" ]]; then
    curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | sh -s -- -b ./bin v0.18.3
fi
./bin/trivy image "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}_dev"

if [[ ! -f "./bin/syft" ]]; then
    curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b ./bin
fi
./bin/syft "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}_dev" -o spdx-json=sbom.spdx.json

if [[ ! -f "./bin/grype" ]]; then
    curl -sSfL https://raw.githubusercontent.com/anchore/grype/main/install.sh | sh -s -- -b ./bin
fi
echo grype scan
./bin/grype --add-cpes-if-none --fail-on critical sbom.spdx.json |awk '{if (NR>1) {print $NF,$0}}' |sort |cut -f2- -d' '

# docker run --rm -it --privileged --mount type=bind,source='/sys/kernel/security',target='/sys/kernel/security' --mount type=bind,source='/etc',target='/etc' --name kapparmor  ghcr.io/tuxerrante/kapparmor:${APP_VERSION}_dev
