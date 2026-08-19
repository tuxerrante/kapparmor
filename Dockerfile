# --- build stage
FROM golang:1.25.3-alpine3.22@sha256:aee43c3ccbf24fdffb7295693b6e33b21e01baec1b2a55acc351fde345e9ec34 AS builder

WORKDIR /builder

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum ./
COPY src/ ./src/

RUN apk update &&\
    apk add --no-cache git   &&\
    cd src/app &&\
    go get    -v .           &&\
    go build  -v -o /go/bin/app .

# Run fuzzing tests only on the main package (not on sub-packages like metrics)
RUN cd /builder/src/app &&\
    go test -v -vet off -fuzz=Fuzz -fuzztime=60s -run ^t_fuzz* .

# Run coverage tests on all packages
ARG MIN_COVERAGE=69
RUN cd /builder/src/app &&\
    go test -v -coverprofile=coverage.out -covermode=count ./... &&\
    go tool cover -func=coverage.out > coverage-summary.txt &&\
    coverage="$(awk '/^total:/ {sub(/%/, "", $3); print $3}' coverage-summary.txt)" &&\
    rm coverage-summary.txt &&\
    if [ -z "$coverage" ]; then echo "Unable to determine test coverage"; exit 1; fi &&\
    awk -v coverage="$coverage" -v minimum="$MIN_COVERAGE" 'BEGIN {\
      printf "Total coverage: %.1f%% (minimum: %.1f%%)\n", coverage, minimum;\
      if (coverage < minimum) exit 1;\
    }'


# --- Publish test coverage results
FROM scratch AS test-coverage
COPY --from=builder /builder/src/app/coverage.out .


# --- Production image
FROM ubuntu:25.10@sha256:7cc5e35f6567ee8c66d2abb4aab0fd866669e6207c237c3a8f0947a5c7f17092
LABEL Name=kapparmor
LABEL Author="Affinito Alessandro"

WORKDIR /app

RUN apt-get update &&\
    apt-get upgrade -y &&\
    apt-get install --no-install-recommends --yes apparmor apparmor-utils &&\
    rm -rf /var/lib/apt/lists/* &&\
    mkdir --parent --verbose /etc/apparmor.d/custom /app/profiles/

COPY --from=builder /go/bin/app /app/
# COPY ./charts/kapparmor/profiles /app/profiles/

ARG PROFILES_DIR
ARG POLL_TIME

ENV PROFILES_DIR=$PROFILES_DIR
ENV POLL_TIME=$POLL_TIME

USER root
ENTRYPOINT ["./app"]
