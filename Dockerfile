FROM alpine:latest
LABEL Name=kapparmor Version=0.0.1
LABEL Author="Affinito Alessandro"

WORKDIR /app

RUN addgroup --system appgroup &&\
    adduser  --system appuser -G appgroup &&\
    apk --no-cache update &&\
    apk add apparmor

# app is download as an artifact by the pipeline
COPY --chown=appuser:appgroup ./go/bin/app /app/app
COPY --chown=appuser:appgroup ./profiles   /app/profiles

RUN chmod 550 app

ARG PROFILES_DIR
ARG POLL_TIME

ENV PROFILES_DIR=$PROFILES_DIR
ENV POLL_TIME=$POLL_TIME

USER appuser
CMD ./app