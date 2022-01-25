#!/bin/sh
set -e

# this script will start relayproxy

PROGNAME=$(basename $0)

USER="bloxroute"
GROUP="bloxroute"
WORKDIR="/app/bloxroute"
STARTUP="gateway $@"

echo "$PROGNAME: Starting $STARTUP"
if [[ "$(id -u)" = '0' ]]; then
    # if running as root, chown and step-down from root
    find . \! -type l \! -user ${USER} -exec chown ${USER}:${GROUP} '{}' +
    cd ${WORKDIR}
    exec su-exec ${USER} ${STARTUP}
else
    # allow the container to be started with `--user`, in this case we cannot use su-exec
    cd ${WORKDIR}
    exec ${STARTUP}
fi
