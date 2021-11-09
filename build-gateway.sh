#!/usr/bin/env bash
IMAGE=033969152235.dkr.ecr.us-east-1.amazonaws.comgateway-go:${1:-latest}
echo "Building container... $IMAGE"
docker build . -f Dockerfile-gateway --rm=true -t $IMAGE
