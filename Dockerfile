ARG GO_VERSION=alpine
ARG BASE=golang:1.19-alpine

FROM ${BASE} as builder

RUN apk update \
 && apk add --no-cache \
    linux-headers \
    gcc \
    libtool \
    openssl-dev \
    libffi \
    tini \
    git \
    'su-exec>=0.2' \
 && apk add --no-cache --virtual .build_deps build-base libffi-dev

# add our user and group first to make sure their IDs get assigned consistently, regardless of whatever dependencies get added
RUN addgroup -g 502 -S bloxroute \
 && adduser -u 502 -S -G bloxroute bloxroute \
 && mkdir -p /app/bloxroute/logs \
 && chown -R bloxroute:bloxroute /app/bloxroute

# Move to working directory
WORKDIR /app/bloxroute

# Download dependency using go mod
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY --chown=bloxroute:bloxroute . .

RUN make gateway
RUN chown bloxroute:bloxroute ./bin/gateway
RUN chown bloxroute:bloxroute ./bin/bxcli

FROM golang:${GO_VERSION}

RUN apk update \
 && apk add --no-cache \
    linux-headers \
    gcc \
    libtool \
    openssl-dev \
    libffi \
    tini \
    git \
    'su-exec>=0.2' \
 && apk add --no-cache --virtual .build_deps build-base libffi-dev

# add our user and group first to make sure their IDs get assigned consistently, regardless of whatever dependencies get added
RUN addgroup -g 502 -S bloxroute \
 && adduser -u 502 -S -G bloxroute bloxroute \
 && mkdir -p /app/bloxroute/logs \
 && mkdir -p /app/bloxroute/datadir \
 && chown -R bloxroute:bloxroute /app/bloxroute

# Move to working directory
WORKDIR /app/bloxroute
RUN chmod +s /bin/ping
RUN chmod +s /bin/busybox

COPY --from=builder /app/bloxroute/bin/bxcli /app/bloxroute/bin/bxcli
COPY --from=builder /app/bloxroute/bin/gateway /app/bloxroute/bin/gateway

COPY docker-entrypoint.sh /usr/local/bin/

ENV PATH="/app/bloxroute/bin:${PATH}"

EXPOSE 1801 5001

ENTRYPOINT ["/sbin/tini", "--", "/bin/sh", "/usr/local/bin/docker-entrypoint.sh"]
