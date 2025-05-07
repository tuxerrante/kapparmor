#!/bin/bash
set -ex

source ./config/config
export BUILD_TIME=$(date +"%Y-%m-%dT%H:%M:%S%:z")
export BUILD_VERSION=${APP_VERSION}_dev
VCS_REF=$(git rev-parse --short HEAD)

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
    docker run --rm gcr.io/go-containerregistry/crane digest golang:1.24.2
    exit 1
fi

# Update Helm index
# envsubst < charts/kapparmor/templates/index.yaml > output/index.yaml
rm -rf output/*.tgz
helm package --app-version ${APP_VERSION} --version ${CHART_VERSION} --destination output/ charts/kapparmor/
helm repo index output/ --merge charts/index.yaml --url https://tuxerrante.github.io/kapparmor/
mv output/index.yaml charts/index.yaml

# Clean old images
# echo "> Removing old and dangling old images..."
# docker rmi "$(docker images --filter "reference=ghcr.io/tuxerrante/kapparmor" -q --no-trunc)" || true

# Clean go module cache
# go clean -modcache
go clean ./go/src/app/...

# Lint and try fixing
if [[ ! -f "./bin/golangci-lint" ]]; then
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin
fi
echo "> Linting go code"
./bin/golangci-lint run --fast-only --allow-parallel-runners ./go/src/app

echo "> Linting the Helm chart"
helm lint --debug --strict charts/kapparmor/

#### To run it look into docs/testing.md
echo "> Building container image..."
if ! docker buildx inspect kapparmor-builder &>/dev/null; then
    docker buildx create --use --name kapparmor-builder --driver docker-container
else
    docker buildx use kapparmor-builder
fi

# Build and push with buildx
docker buildx build \
    --platform linux/amd64 \
    --tag "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}_dev" \
    --build-arg POLL_TIME=30 \
    --build-arg PROFILES_DIR=/app/profiles \
    --cache-from type=registry,ref=ghcr.io/tuxerrante/kapparmor:cache \
    --cache-to type=registry,ref=ghcr.io/tuxerrante/kapparmor:cache,mode=max \
    --provenance mode=max \
    --sbom true \
    --progress=plain \
    --push \
    --label "org.opencontainers.image.created=${BUILD_TIME}" \
    --label "org.opencontainers.image.version=${BUILD_VERSION}" \
    --label "org.opencontainers.image.revision=${VCS_REF}" \
    -f Dockerfile \
    .

if [[ ! -f "./bin/trivy" ]]; then
    curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | sudo sh -s -- -b /usr/bin v0.62.1
fi
sudo /usr/bin/trivy image "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}_dev" >output/trivy.log

if [[ ! -f "./bin/syft" ]]; then
    curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b ./bin
fi
./bin/syft "ghcr.io/tuxerrante/kapparmor:${APP_VERSION}_dev" -o spdx-json=sbom.spdx.json

if [[ ! -f "./bin/grype" ]]; then
    curl -sSfL https://raw.githubusercontent.com/anchore/grype/main/install.sh | sh -s -- -b ./bin
fi
echo grype scan
./bin/grype --add-cpes-if-none --fail-on critical sbom.spdx.json | awk '{if (NR>1) {print $NF,$0}}' | sort | cut -f2- -d' ' >output/grype.log

# docker run --rm -it --privileged --mount type=bind,source='/sys/kernel/security',target='/sys/kernel/security' --mount type=bind,source='/etc',target='/etc' --name kapparmor  ghcr.io/tuxerrante/kapparmor:${APP_VERSION}_dev
