package rpc

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type blxrCredentials struct {
	authorization string
}

func (bc blxrCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": bc.authorization,
	}, nil
}

func (bc blxrCredentials) RequireTransportSecurity() bool {
	return false
}

// NewBLXRCredentials constructs a new bloxroute GRPC auth scheme from a raw auth header
func NewBLXRCredentials(authorization string) grpc.DialOption {
	return grpc.WithPerRPCCredentials(blxrCredentials{authorization: authorization})
}

// NewBLXRCredentialsFromUserPassword constructs a new bloxroute GRPC auth scheme from an RPC user and secret
func NewBLXRCredentialsFromUserPassword(user string, secret string) grpc.DialOption {
	return grpc.WithPerRPCCredentials(blxrCredentials{authorization: EncodeUserSecret(user, secret)})
}

// EncodeUserSecret produces a base64 encoded auth header of a user and secret
func EncodeUserSecret(user string, secret string) string {
	data := fmt.Sprintf("%v:%v", user, secret)
	return base64.StdEncoding.EncodeToString([]byte(data))
}

// DecodeAuthHeader produces the user and secret from an encoded auth header
func DecodeAuthHeader(authHeader string) (user string, secret string, err error) {
	data, err := base64.StdEncoding.DecodeString(authHeader)
	if err != nil {
		return
	}

	splitData := strings.Split(string(data), ":")
	if len(splitData) != 2 {
		err = fmt.Errorf("improperly formatted decoded auth header: %v", string(data))
		return
	}

	user = splitData[0]
	secret = splitData[1]
	return
}

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
