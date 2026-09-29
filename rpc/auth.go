package rpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/metadata"
)

// ReadAuthMetadata reads auth info from the RPC connection context
func ReadAuthMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("could not read metadata from context")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return "", errors.New("no auth information was provided")
	}
	return values[0], nil
}
