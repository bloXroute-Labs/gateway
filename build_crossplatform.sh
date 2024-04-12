#!/usr/bin/env bash
set -e

# Build docker image for linux/amd64 and linux/arm64 platforms
# For MacOS requires:
#   brew install filosottile/musl-cross/musl-cross

IMAGE=bloxroute/bxgateway-go-release-candidate:${1:-latest}
PLATFORM=${2:-linux/amd64}
CUR_PLATFORM="$(go env GOOS)/$(go env GOARCH)"

GOOS=${PLATFORM%/*}
GOARCH=${PLATFORM#*/}

export GOOS
export GOARCH

if [[ $PLATFORM == "$CUR_PLATFORM" ]]; then
    echo "Building for local platform"
elif [[ $GOARCH == "arm64" ]]; then
    ARCH="aarch64"

    export CC="${ARCH}-${GOOS}-musl-gcc"
    export CXX="${ARCH}-${GOOS}-musl-g++"
    export CGO_ENABLED="1"

elif [[ $GOARCH == "amd64" ]]; then
    ARCH="x86_64"

    export CC="${ARCH}-${GOOS}-musl-gcc"
    export CXX="${ARCH}-${GOOS}-musl-g++"
    export CGO_ENABLED="1"
else
    echo "Error: Unsupported ${GOARCH} architecture"
    exit 1
fi

make gateway

# extract tag and keep in file (can't be done in docker). Used by Makefile
tag=$(git describe --tags --always --dirty --match=v2* 2> /dev/null)
echo "$tag" > .gittag
echo "Building container... $IMAGE"
docker build . -f Dockerfile.localbin --rm=true --platform "$PLATFORM" -t "$IMAGE"
rm .gittag