#!/usr/bin/env bash
# This `run_gateway.sh` script can be used to run a bloXroute Go Light gateway.
# The script takes five arguments
# CERT_PATH: Your SSL certificates from the portal. This should be a path to
# the parent directory of the external_gateway folder.
# The external_gateway folder should contain a registration_only folder with certs inside
# LOGS_PATH: Where you want a logs folder to be created with a gateway.log file inside
# DATADIR_PATH: The directory for storing various persistent files such as gateway private key file
# BLOCKCHAIN_NETWORK: available options are Mainnet (for Ethereum), BSC-Mainnet, Polygon-Mainnet
# ENODES: Local blockchain node, Enterprise clients must start the gateway with local node connection

# Example usage:
# There are two ways to start a Go Gateway with this script.
# 1. Specify input arguments at startup:
# `./run_gateway.sh /home/ubuntu/gw/ssl /home/ubuntu/gw/logs /home/ubuntu/gw/datadir Mainnet enode://123..abc@127.0.0.1:30303`
# 2. Modify line 25-29 to add correct values, then run command:
# `./run_gateway.sh`
#
# It creates a container named -go running in the background
# Websockets are accessible at ws://localhost:28333/ws
# To test it you can use wscat -c ws://localhost:28333/ws -H "Authorization:<your auth header here>"

CERT_PATH=${1:-/home/ec2-user/ssl}
LOGS_PATH=${2:-/home/ec2-user/logs}
DATADIR_PATH=${3:-/home/ec2-user/datadir}
BLOCKCHAIN_NETWORK=${4:-"Mainnet"}
ENODES=${5:-""}

LOG_LEVEL="info"
LOG_FILE_LEVEL="debug"
IMAGE_TAG="latest"
EXTERNAL_IP=""

CA_CERT_URL="https://s3.amazonaws.com/credentials.blxrbdn.com/ca/ca_cert.pem"
mkdir -p "$CERT_PATH"/external_gateway/ca
curl $CA_CERT_URL -o "$CERT_PATH"/external_gateway/ca/ca_cert.pem

ARGS="--ws --port 1801"
if [[ "${EXTERNAL_IP}" != "" ]]; then
  ARGS="${ARGS} --external-ip ${EXTERNAL_IP}"
fi

if [[ "${LOG_LEVEL}" != "" ]]; then
  ARGS="${ARGS} --log-level ${LOG_LEVEL}"
fi

if [[ "${LOG_FILE_LEVEL}" != "" ]]; then
   ARGS="${ARGS} --log-file-level ${LOG_FILE_LEVEL}"
fi

if [[ "${ENODES}" != "" ]]; then
  ARGS="${ARGS} --enodes ${ENODES}"
fi

ARGS="${ARGS} --blockchain-network ${BLOCKCHAIN_NETWORK}"

docker pull bloxroute/gateway:$IMAGE_TAG
docker rm -f -go
docker run --name -go -d -v "$LOGS_PATH":/app/bloxroute/logs \
  -v "$CERT_PATH":/app/bloxroute/ssl -v "$DATADIR_PATH":/app/bloxroute/datadir \
  -p 1801:1801 -p 127.0.0.1:6060:6060 \
  -p 28333:28333 bloxroute/gateway:$IMAGE_TAG "$ARGS"