#!/usr/bin/env bash
### ./publish_gateway develop v2.104.5

VERSION=${1}

BRANCH="release-candidate-${VERSION}"
git checkout -b ${BRANCH}

cd ../bxgateway-private-go
git checkout develop
git pull origin develop
cd ../bloxroute-gateway-go

echo "copying bxgateway folder and files"
cp -r ../bxgateway-private-go/bxgateway .
cp -r ../bxgateway-private-go/test .
cp ../bxgateway-private-go/.dockerignore .
cp ../bxgateway-private-go/.gitignore .
cp ../bxgateway-private-go/docker-entrypoint-gateway.sh .
cp ../bxgateway-private-go/Dockerfile-gateway .
cp ../bxgateway-private-go/Makefile .
cp ../bxgateway-private-go/README.md .
cp ../bxgateway-private-go/build-gateway.sh .

echo "updating branch"
git add .
git commit -m "update repository for publish"
git push origin ${BRANCH}

echo "creating tag"
git tag ${VERSION}
git push origin ${VERSION}

