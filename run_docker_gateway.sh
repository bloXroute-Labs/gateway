#!/usr/bin/env bash
BRANCH=${1:-develop}
ENV=${2:-testnet}
TYPE=${3:-relay}

EXTERNAL_IP=$(dig +short myip.opendns.com @resolver1.opendns.com)
[ ! -z ${EXTERNAL_IP} ] && EXTERNAL_IP_ARG="--external-ip ${EXTERNAL_IP}" || EXTERNAL_IP_ARG=""

DOCKER_TAG=${BRANCH/\//-}

if [[ "${TYPE}" == "relay" ]]; then
  aws s3 cp s3://files.bloxroute.com/proxy/${ENV}/peers /home/ec2-user/proxypeers
  mkdir -p /home/ec2-user/datadir

  CONTINENT=${BXMETADATA_continent:+"--continent \"$BXMETADATA_continent\""}
  REGION=${BXMETADATA_region:+"--region \"$BXMETADATA_region\""}
  COUNTRY=${BXMETADATA_country:+"--country \"$BXMETADATA_country\""}

  ./build.sh $DOCKER_TAG

  docker rm -f bxrelay-proxy
  docker run --name bxrelay-proxy -d \
    -m 5g \
    --restart unless-stopped \
    -v /home/ec2-user/gateway/logs:/app/bxrelay-proxy/logs \
    -v /home/ec2-user/ssl:/app/bxrelay-proxy/ssl \
    -v /home/ec2-user/proxypeers:/app/bxrelay-proxy/proxypeers \
    -v /home/ec2-user/datadir:/app/bxrelay-proxy/datadir \
    -p 1810:1810 \
    -p 127.0.0.1:6060:6060 \
    -p 127.0.0.1:5002:5002 \
    -p 28337:28337 \
    033969152235.dkr.ecr.us-east-1.amazonaws.com/bxrelay-proxy:$DOCKER_TAG \
    --env $ENV \
    --relay-block-host 172.17.0.1 \
    --relay-tx-host 172.17.0.1 \
    --fluentd \
    --fluentd-host 172.17.0.1 \
    --ws-host 0.0.0.0 \
    --ws-port 28337 \
    --grpc \
    --grpc-host 0.0.0.0 \
    ${EXTERNAL_IP_ARG} \
    --peer-file proxypeers \
    ${CONTINENT} ${REGION} ${COUNTRY}
else
  ./build-gateway.sh $DOCKER_TAG

  docker rm -f bxgateway-go
  docker run --name bxgateway-go -d \
  -v /home/ec2-user/gateway/logs:/app/bloxroute/logs \
  -p 1801:1801 \
  -p 127.0.0.1:6060:6060 \
  -p 127.0.0.1:5001:5001 \
  -p 28337:28337 \
  bloxroute/bloxroute-gateway-go:$DOCKER_TAG \
  --env $ENV \
  --grpc \
  --grpc-host 0.0.0.0 \
  ${EXTERNAL_IP_ARG}
fi
