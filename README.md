# kapparmor
apparmor-loader project to deploy profiles through a kubernetes daemonset

## History

# Prerequisites
helm create kapparmor

sudo usermod -aG docker $USER

# Create mod files in root dir
go mod init github.com/tuxerrante/kapparmor
go mod init ./go/src/app/

# Build and run the container image
docker build -t mygoapp --build-arg POLL_TIME=30 --build-arg PROFILES_DIR=/app/profiles -f Dockerfile . &&\
 echo &&\
 docker run --rm -it mygoapp /bin/sh

