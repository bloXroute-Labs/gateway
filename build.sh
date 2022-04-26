#!/usr/bin/env bash
IMAGE=bloxroute/gateway:${1:-latest}
echo "Building container... $IMAGE"
docker build . -f Dockerfile --rm=true -t $IMAGE
