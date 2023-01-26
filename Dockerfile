# --- build stage
FROM golang:1.19-alpine AS builder
RUN apk add --no-cache git
WORKDIR /go/src/app
COPY . .
RUN go get -d -v ./go/src/app/
RUN go build -o /go/bin/app -v ./go/src/app/

# ---
FROM alpine:latest
LABEL Name=kapparmor Version=0.0.1
LABEL Author="Affinito Alessandro"

WORKDIR /app

RUN apk --no-cache update           &&\
    apk add apparmor libapparmor    &&\
    mkdir --parent --verbose /etc/apparmor.d/custom /sys/kernel/security/apparmor

COPY --from=builder ./go/bin/app /app/
COPY ./charts/kapparmor/profiles   /app/profiles

ARG PROFILES_DIR
ARG POLL_TIME

ENV PROFILES_DIR=$PROFILES_DIR
ENV POLL_TIME=$POLL_TIME

USER root
CMD ./app