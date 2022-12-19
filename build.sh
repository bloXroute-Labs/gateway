#!/usr/bin/env bash
IMAGE=bloxroute/gateway:${1:-latest}
# extract tag and keep in file (can't be done in docker). Used by Makefile
tag=`git describe --tags --always --dirty --match=v2* 2> /dev/null`
echo $tag > .gittag
echo "Building container... $IMAGE"
docker build --no-cache . -f Dockerfile --rm=true --platform linux/x86_64 -t $IMAGE
rm .gittag