# --- build stage
FROM golang:1.26.1-alpine3.22@sha256:07e91d24f6330432729082bb580983181809e0a48f0f38ecde26868d4568c6ac AS builder

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
RUN cd /builder/src/app &&\
    go test -v -coverprofile=coverage.out -covermode=count ./...


# --- Publish test coverage results
FROM scratch AS test-coverage
COPY --from=builder /builder/src/app/coverage.out .


# --- Production image
FROM ubuntu:24.04@sha256:0d39fcc8335d6d74d5502f6df2d30119ff4790ebbb60b364818d5112d9e3e932
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
