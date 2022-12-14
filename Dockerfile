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

RUN addgroup --system appgroup &&\
    adduser  --system appuser -G appgroup &&\
    apk --no-cache update &&\
    apk add apparmor

COPY --chown=appuser:appgroup --from=builder ./go/bin/app /app/
COPY --chown=appuser:appgroup ./profiles   /app/profiles

RUN chmod 550 app

ARG PROFILES_DIR
ARG POLL_TIME

ENV PROFILES_DIR=$PROFILES_DIR
ENV POLL_TIME=$POLL_TIME

USER appuser
CMD ./app