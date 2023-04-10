FROM golang:1.19
ENV PROTOC_VERSION=3.19.3

RUN apt-get update && apt-get install unzip

RUN curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-x86_64.zip && \
    unzip -o /go/protoc-${PROTOC_VERSION}-linux-x86_64.zip -d /usr/local bin/protoc && \
    unzip -o /go/protoc-${PROTOC_VERSION}-linux-x86_64.zip -d /usr/local include/* && \
    rm -rf protoc-${PROTOC_VERSION}-linux-x86_64.zip

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.26 && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1

RUN mkdir -p /go/protobuf

WORKDIR "/go/protobuf"