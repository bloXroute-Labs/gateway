#!/usr/bin/env bash
# This script can be used to run a bloXroute gateway running with Go.
# The script takes three arguments
# CERT_PATH: Your SSL certificates from the portal. This should be a path to
# the parent directory of the external_gateway folder.
# The external_gateway folder should contain a registration_only folder with certs inside
# LOGS_PATH: Where you want a logs folder to be created with a gateway.log file inside
# EXTERNAL_IP: Optional argument for certain regions of china where you need to set external ip manually
# Example usage:
#
# ./run_docker.sh /home/ubuntu/gw /home/ubuntu/gw/logs
#
# It creates a container named bxgateway-go running in the background
# Websockets are accessible at ws://localhost:28334/ws
# To test them you can use wscat -c ws://localhost:28334/ws -H "Authorization:<your auth header here>"
# If you see Connected, websockets are available

CERT_PATH=${1:-/home/ec2-user/ssl}
LOGS_PATH=${2:-/home/ec2-user/gateway/logs}
BLOCKCHAIN_NETWORK=${3:-"Mainnet"}
EXTERNAL_IP=${4:-""}

# Modify this to hardcode the relay IP you want to connect to
RELAY_IP=""
LOG_LEVEL="debug"
IMAGE_TAG="test"
ENV="testnet"

if [[ ${ENV} == "mainnet" ]]; then
  CA_CERT_URL="https://s3.amazonaws.com/credentials.blxrbdn.com/ca/ca_cert.pem"
else
  CA_CERT_URL="https://s3.amazonaws.com/credentials.bxrtest.com/ca/ca_cert.pem"
fi

mkdir -p "$CERT_PATH"/external_gateway/ca
curl $CA_CERT_URL -o "$CERT_PATH"/external_gateway/ca/ca_cert.pem

ARGS="--ws --port 1802"
if [[ "${EXTERNAL_IP}" != "" ]]; then
  ARGS="${ARGS} --external-ip ${EXTERNAL_IP}"
fi

if [[ "${RELAY_IP}" != "" ]]; then
  ARGS="${ARGS} --relay-ip ${RELAY_IP}"
fi

if [[ "${LOG_LEVEL}" != "" ]]; then
  ARGS="${ARGS} --log-level ${LOG_LEVEL}"
fi

ARGS="${ARGS} --blockchain-network ${BLOCKCHAIN_NETWORK}"

docker pull bloxroute/bloxroute-gateway-go:$IMAGE_TAG
docker rm -f bxgateway-go
docker run --name bxgateway-go -d -v "$LOGS_PATH":/app/bloxroute/logs \
  -v "$CERT_PATH":/app/bloxroute/ssl/${ENV} -p 1802:1802 -p 127.0.0.1:6060:6060 \
  -p 28334:28333 bloxroute/bloxroute-gateway-go:$IMAGE_TAG "$ARGS --env ${ENV}"