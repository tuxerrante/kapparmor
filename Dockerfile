# --- build stage
FROM golang:alpine AS builder
RUN apk add --no-cache git
WORKDIR /go/src/app
COPY . .
RUN go get -d -v ./...
RUN go build -o /go/bin/app -v ./...

# --- final stage
FROM alpine:latest
LABEL Name=kapparmor Version=0.0.1

WORKDIR /app

RUN addgroup --system appgroup &&\
    adduser  --system appuser -G appgroup &&\
    apk --no-cache update

COPY --chown=appuser:appgroup --from=builder /go/bin/app /app
COPY --chown=appuser:appgroup ./profiles ./profiles

ARG PROFILES_DIR
ENV PROFILES_DIR = "/app/profiles"
ARG POLL_TIME
ENV POLL_TIME = 60

# CMD ?
# 000(alex),4(adm),24(cdrom),27(sudo),30(dip),46(plugdev),122(lpadmin),134(lxd),135(sambashare),1001(microk8s)