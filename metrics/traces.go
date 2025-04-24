package metrics

import (
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// ProcessRPCResourceName generates a resource name used with OperationProcessRPC
func ProcessRPCResourceName(rpcMethod string) tracer.StartSpanOption {
	return tracer.ResourceName("Process RPC Request: " + rpcMethod)
}

// ForwardToBuilderResource generates a resource name used with OperationForwardBundleToBuilders
func ForwardToBuilderResource(builderURL string) tracer.StartSpanOption {
	return tracer.ResourceName("Forward to Builder: " + builderURL)
}

// ForwardToMasterNodeResource generates a resource name used with OperationForwardRPCRequestToBackrunMeMasterNodesCallingNode
func ForwardToMasterNodeResource(nodeURL string) tracer.StartSpanOption {
	return tracer.ResourceName("Forward to Master Node: " + nodeURL)
}
