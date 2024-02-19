# --- build stage
FROM golang:1.22.0@sha256:03082deb6ae090a0caa4e4a8f666bc59715bc6fa67f5fd109f823a0c4e1efc2a AS builder

WORKDIR /builder/app
COPY go/src/app/ .
COPY go.mod .

RUN go get    -d -v .           &&\
    go build  -v -o /go/bin/app .

RUN go test -v -vet off -fuzz=Fuzz -fuzztime=60s -run ^t_fuzz* ./...
RUN go test -v -coverprofile=coverage.out -covermode=count ./...


# --- Publish test coverage results
FROM scratch as test-coverage
COPY --from=builder /builder/app/coverage.out .


# --- Production image
FROM ubuntu:latest@sha256:f9d633ff6640178c2d0525017174a688e2c1aef28f0a0130b26bd5554491f0da
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