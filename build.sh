#!/usr/bin/env bash
IMAGE=bloxroute/bloxroute-gateway-go:${1:-latest}
# we should build public version for at least two platforms:
# linux/x86_64 (default) and linux/arm64/v8
PLATFORM=${2:-linux/x86_64}
PORTABLE=${3:-false}
# extract tag and keep in file (can't be done in docker). Used by Makefile
tag=`git describe --tags --always --dirty --match='v2*' 2> /dev/null`
echo $tag > .gittag
if [ "$PORTABLE" = "true" ]; then
    echo "Building portable container with BLST_PORTABLE... $IMAGE-portable"
    docker build --no-cache . -f Dockerfile --rm=true --platform linux/x86_64 --build-arg PORTABLE=true -t $IMAGE
else
    echo "Building container... $IMAGE"
    docker build --no-cache . -f Dockerfile --rm=true --platform $PLATFORM -t $IMAGE
fi

rm .gittag
