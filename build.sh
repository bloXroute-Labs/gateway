#!/usr/bin/env bash
IMAGE=bloxroute/bxgateway-go-release-candidate:${1:-latest}
# extract tag and keep in file (can't be done in docker). Used by Makefile
tag=`git describe --tags --always --dirty --match=v2* 2> /dev/null`
echo $tag > .gittag
echo "Building container... $IMAGE"
docker build  . -f Dockerfile --rm=true --platform linux/x86_64 -t $IMAGE
docker tag $IMAGE 033969152235.dkr.ecr.us-east-1.amazonaws.com/bloxroute-gateway-go:${1:-latest}
docker tag $IMAGE bloxroute/bxgateway-go-release-candidate:${1:-latest}
rm .gittag