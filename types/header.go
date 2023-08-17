package types

import (
	"context"
	"net/http"

	"google.golang.org/grpc/metadata"
)

const (
	// SDKBlockchainHeaderKey is the header key for the blockchain name
	SDKBlockchainHeaderKey = "X-BloXroute-Blockchain"

	// SDKVersionHeaderKey is the header key for the SDK version
	SDKVersionHeaderKey = "X-BloXroute-SDK-Version"

	// SDKCodeLanguageHeaderKey is the header key for the programming language SDK code is written in
	SDKCodeLanguageHeaderKey = "X-BloXroute-Code-Language"
)

// SDKMetaFromHeaders converts HTTP SDK headers to a meta map
func SDKMetaFromHeaders(headers http.Header) map[string]string {
	meta := make(map[string]string)
	meta[SDKBlockchainHeaderKey] = headers.Get(SDKBlockchainHeaderKey)
	meta[SDKVersionHeaderKey] = headers.Get(SDKVersionHeaderKey)
	meta[SDKCodeLanguageHeaderKey] = headers.Get(SDKCodeLanguageHeaderKey)
	return meta
}

// SDKMetaFromContext converts gRPC SDK metadata to a meta map
func SDKMetaFromContext(ctx context.Context) map[string]string {
	meta := make(map[string]string)

	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if blockchainHeader := md.Get(SDKBlockchainHeaderKey); len(blockchainHeader) > 0 {
			meta[SDKBlockchainHeaderKey] = blockchainHeader[0]
		}
		if sourceCodeHeader := md.Get(SDKCodeLanguageHeaderKey); len(sourceCodeHeader) > 0 {
			meta[SDKCodeLanguageHeaderKey] = sourceCodeHeader[0]
		}
		if sdkVersionHeader := md.Get(SDKVersionHeaderKey); len(sdkVersionHeader) > 0 {
			meta[SDKVersionHeaderKey] = sdkVersionHeader[0]
		}
	}

	return meta
}
