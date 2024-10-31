package rpc

import (
	"context"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/config"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"google.golang.org/grpc"
)

// AuthOption parses authentication info from the provided CLI context
func AuthOption(grpcConfig *config.GRPC) (authOption grpc.DialOption, included bool) {
	if grpcConfig.AuthEnabled {
		included = true

		if grpcConfig.EncodedAuthSet {
			authOption = NewBLXRCredentials(grpcConfig.EncodedAuth)
		} else {
			authOption = NewBLXRCredentialsFromUserPassword(grpcConfig.User, grpcConfig.Password)
		}
	}
	return
}

func connectInsecure(grpcConfig *config.GRPC) (*grpc.ClientConn, error) {
	address := fmt.Sprintf("%v:%v", grpcConfig.Host, grpcConfig.Port)
	authOption, required := AuthOption(grpcConfig)

	if required {
		return grpc.Dial(address, grpc.WithInsecure(), authOption)
	}
	return grpc.Dial(address, grpc.WithInsecure())
}

// GatewayClient returns a ready-to-use GRPC gateway client
func GatewayClient(grpcConfig *config.GRPC) (pb.GatewayClient, error) {
	conn, err := connectInsecure(grpcConfig)

	if err != nil {
		return nil, fmt.Errorf("could not connect to gateway GRPC: %v", err)
	}
	pbConn := pb.NewGatewayClient(conn)
	return pbConn, nil
}

// GatewayCall executes a GRPC gateway call
func GatewayCall(grpcConfig *config.GRPC, call func(ctx context.Context, client pb.GatewayClient) (interface{}, error)) (interface{}, error) {
	client, err := GatewayClient(grpcConfig)
	if err != nil {
		return nil, err
	}

	callContext, cancel := context.WithTimeout(context.Background(), grpcConfig.Timeout)
	defer cancel()

	return call(callContext, client)
}

// GatewayConsoleCall executes a GRPC gateway call and logs the output to stdout as JSON
func GatewayConsoleCall(grpcConfig *config.GRPC, call func(ctx context.Context, client pb.GatewayClient) (interface{}, error)) error {
	result, err := GatewayCall(grpcConfig, call)
	if err != nil {
		return err
	}

	msg, ok := result.(proto.Message)
	if !ok {
		return fmt.Errorf("result is not a protobuf message")
	}

	jsm := &protojson.MarshalOptions{
		Indent:        "    ",
		UseProtoNames: true, // Use the original snake case name of the fields
	}

	jstr, err := jsm.Marshal(msg)
	if err != nil {
		return fmt.Errorf("could not marshal JSON: %v", err)
	}
	fmt.Println(string(jstr))
	return nil
}
