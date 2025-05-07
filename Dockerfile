# --- build stage
FROM golang:1.24.2@sha256:30baaea08c5d1e858329c50f29fe381e9b7d7bced11a0f5f1f69a1504cdfbf5e AS builder

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
FROM scratch AS test-coverage
COPY --from=builder /builder/app/coverage.out .


# --- Production image
FROM ubuntu:24.04@sha256:1e622c5f073b4f6bfad6632f2616c7f59ef256e96fe78bf6a595d1dc4376ac02
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