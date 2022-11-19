# --- build stage
FROM golang:alpine AS builder
RUN apk add --no-cache git
WORKDIR /go/src/app
COPY . .
RUN go get -d -v ./go/src/app/
RUN go build -o /go/bin/app -v ./go/src/app/

# --- final stage ---
FROM alpine:latest
LABEL Name=kapparmor Version=0.0.1

WORKDIR /app

RUN addgroup --system appgroup &&\
    adduser  --system appuser -G appgroup &&\
    apk --no-cache update

COPY --chown=appuser:appgroup --from=builder /go/bin/app /app
COPY --chown=appuser:appgroup ./profiles ./profiles

ARG PROFILES_DIR
ARG POLL_TIME

ENV PROFILES_DIR=$PROFILES_DIR
ENV POLL_TIME=$POLL_TIME

# USER appuser
ENTRYPOINT ./app