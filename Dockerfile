# --- build stage
FROM golang:1.22.5@sha256:829eff99a4b2abffe68f6a3847337bf6455d69d17e49ec1a97dac78834754bd6 AS builder

WORKDIR /builder/app
COPY go/src/app/ .
COPY go/src/tests/ /builder/tests/
COPY go.mod .

RUN go get    -d -v .           &&\
    go mod tidy &&\
    go build  -v -o /go/bin/app .

RUN go test -v -coverprofile=coverage.out -covermode=atomic ./...
#    go tool cover -func=coverage.out


# --- Publish test coverage results
FROM scratch as test-coverage
COPY --from=builder /builder/app/coverage.out .


# --- Production image
FROM ubuntu:24.04@sha256:2e863c44b718727c860746568e1d54afd13b2fa71b160f5cd9058fc436217b30
LABEL Name=kapparmor
LABEL Author="Affinito Alessandro"

WORKDIR /app

RUN apt-get update &&\
    apt-get upgrade -y &&\
    apt-get install --no-install-recommends --yes apparmor apparmor-utils &&\
    rm -rf /var/lib/apt/lists/* &&\
    mkdir --parent --verbose /etc/apparmor.d/custom 

COPY --from=builder /go/bin/app /app/
COPY ./charts/kapparmor/profiles /app/profiles/

ARG PROFILES_DIR
ARG POLL_TIME

ENV PROFILES_DIR=$PROFILES_DIR
ENV POLL_TIME=$POLL_TIME

USER root
ENTRYPOINT ["./app"]