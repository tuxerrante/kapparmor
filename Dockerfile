FROM alpine:latest
LABEL Name=kapparmor Version=0.0.1
LABEL Author="Affinito Alessandro"

WORKDIR /app

# app is download as an artifact by the pipeline
COPY --chown=appuser:appgroup ./go/bin/app /app/app
COPY --chown=appuser:appgroup ./profiles   /app/profiles

RUN addgroup --system appgroup &&\
    adduser  --system appuser -G appgroup &&\
    apk --no-cache update &&\
    apk add apparmor &&\
    chmod 550 app &&\
    ls -lah

ARG PROFILES_DIR
ARG POLL_TIME

ENV PROFILES_DIR=$PROFILES_DIR
ENV POLL_TIME=$POLL_TIME

USER appuser
CMD ./app