package handler

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// RPCCall represents customer call executed for onBlock Feed
type RPCCall struct {
	commandMethod string
	blockOffset   int
	callName      string
	callPayload   map[string]string
	active        bool
}

// FillCalls fill calls
func FillCalls(nodeWSManager blockchain.WSManager, calls map[string]*RPCCall, callIdx int, callParams map[string]string) error {
	call := newCall(strconv.Itoa(callIdx))
	err := call.constructCall(callParams, nodeWSManager)
	if err != nil {
		return err
	}
	_, nameExists := calls[call.callName]
	if nameExists {
		return fmt.Errorf("unique name must be provided for each call: call %v already exists", call.callName)
	}
	calls[call.callName] = call
	return err
}

func newCall(name string) *RPCCall {
	return &RPCCall{
		callName:    name,
		callPayload: make(map[string]string),
		active:      true,
	}
}

func (c *RPCCall) constructCall(callParams map[string]string, nodeWSManager blockchain.WSManager) error {
	for param, value := range callParams {
		switch param {
		case "method":
			isValidMethod := utils.Exists(value, nodeWSManager.ValidRPCCallMethods())
			if !isValidMethod {
				return fmt.Errorf("invalid method %v provided. Supported methods: %v", value, nodeWSManager.ValidRPCCallMethods())
			}
			c.commandMethod = value
		case "tag":
			if value == "latest" {
				c.blockOffset = 0
				break
			}
			blockOffset, err := strconv.Atoi(value)
			if err != nil || blockOffset > 0 {
				return fmt.Errorf("invalid value %v provided for tag. Supported values: latest, 0 or a negative number", value)
			}
			c.blockOffset = blockOffset
		case "name":
			c.callName = value
		default:
			isValidPayloadField := utils.Exists(param, nodeWSManager.ValidRPCCallPayloadFields())
			if !isValidPayloadField {
				return fmt.Errorf("invalid payload field %v provided. Supported fields: %v", param, nodeWSManager.ValidRPCCallPayloadFields())
			}
			c.callPayload[param] = value
		}
	}
	requiredFields, ok := nodeWSManager.RequiredPayloadFieldsForRPCMethod(c.commandMethod)
	if !ok {
		return fmt.Errorf("unexpectedly, unable to find required fields for method %v", c.commandMethod)
	}
	err := c.validatePayload(c.commandMethod, requiredFields)
	if err != nil {
		return err
	}
	return nil
}

func (c *RPCCall) validatePayload(method string, requiredFields []string) error {
	for _, field := range requiredFields {
		_, ok := c.callPayload[field]
		if !ok {
			return fmt.Errorf("expected %v element in request payload for %v", field, method)
		}
	}
	return nil
}

func (c *RPCCall) string() string {
	payloadBytes, err := json.Marshal(c.callPayload)
	if err != nil {
		log.Errorf("failed to convert eth call to string: %v", err)
		return c.callName
	}

	return fmt.Sprintf("%+v", struct {
		commandMethod string
		blockOffset   int
		callName      string
		callPayload   string
		active        bool
	}{
		commandMethod: c.commandMethod,
		blockOffset:   c.blockOffset,
		callName:      c.callName,
		callPayload:   string(payloadBytes),
		active:        c.active,
	})
}
