# --- build stage
# --- Done by the pipeline/workflow
# FROM golang:1.19-alpine AS builder
# RUN apk add --no-cache git
# WORKDIR /go/src/app
# COPY . .
# RUN go get -d -v ./go/src/app/
# RUN go build -o /go/bin/app -v ./go/src/app/

# --- final stage ---
FROM alpine:latest
LABEL Name=kapparmor Version=0.0.1
LABEL Author="Affinito Alessandro"

WORKDIR /app

RUN set +x &&\
    addgroup --system appgroup &&\
    adduser  --system appuser -G appgroup &&\
    apk --no-cache update &&\
    apk add apparmor &&\
    ls -lah

COPY --chown=appuser:appgroup app /app
COPY --chown=appuser:appgroup ./profiles ./profiles

ARG PROFILES_DIR
ARG POLL_TIME

ENV PROFILES_DIR=$PROFILES_DIR
ENV POLL_TIME=$POLL_TIME

# USER appuser
CMD ./app