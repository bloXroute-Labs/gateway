#!/usr/bin/env bash

# - expand -xyz into -x -y -z
# - expand --longopt=arg into --longopt arg
ARGS=()
END=
while [[ $# -gt 0 ]]; do
  arg="$1"; shift
  case "${END}${arg}" in
    --) ARGS+=("$arg"); END=1 ;;
    --*=*) ARGS+=("${arg%%=*}" "${arg#*=}") ;;
    --*) ARGS+=("$arg") ;;
    -*) for i in $(seq 2 ${#arg}); do ARGS+=("-${arg:i-1:1}"); done ;;
    *) ARGS+=("$arg") ;;
  esac
done

set -- "${ARGS[@]}"

# defaults
CONTAINER_NAME="bxgateway-go"
GRPC_HOST="127.0.0.1"
GRPC_PORT=5001

while [[ $# -gt 0 ]]; do
  case $1 in
    --host) GRPC_HOST="$3"; shift; shift ;;
    --port) GRPC_PORT="$2"; shift; shift ;;
    --container) CONTAINER_NAME="$2"; shift; shift ;;
    -*) echo "illegal option $1"; exit 1 ;;
    *) break ;;
  esac
done

if ! docker inspect "$CONTAINER_NAME" > /dev/null 2>&1; then
  echo "container $CONTAINER_NAME is not running"
  exit 1
fi

docker exec "$CONTAINER_NAME" bash -c "cd /app/bloxroute/bin; ./bxcli --grpc-host $GRPC_HOST --grpc-port $GRPC_PORT status"