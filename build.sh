#!/usr/bin/env bash
IMAGE=bloxroute/gateway:${1:-latest}
# we should build public version for at least two platforms:
# linux/x86_64 (default) and linux/arm64/v8 ${2:linux/x86_64}
PLATFORM=${2:-linux/x86_64}
# extract tag and keep in file (can't be done in docker). Used by Makefile
tag=`git describe --tags --always --dirty --match='v2*' 2> /dev/null`
echo $tag > .gittag
echo "Building container... $IMAGE"
docker build --no-cache . -f Dockerfile --rm=true --platform $PLATFORM -t $IMAGE
rm .gittag
