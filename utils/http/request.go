package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sourcegraph/jsonrpc2"
)

const postRequestTimeout = 10 * time.Second

// NewPOSTRequest servers as a generic post request handler and accepts Authorization token optionally
func NewPOSTRequest[T any](client *http.Client, endpoint string, params any, authToken string) (T, error) {
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return *new(T), fmt.Errorf("failed to marshal params, error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), postRequestTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(paramsBytes))
	if err != nil {
		err := fmt.Errorf("failed to create http request for %v, with params %v, error %v, ", endpoint, params, err)
		return *new(T), err
	}
	request.Header.Add("Accept", "application/json")
	request.Header.Add("Content-Type", "application/json")

	if authToken != "" {
		request.Header.Add("Authorization", authToken)
	}

	response, err := client.Do(request)
	if err != nil {
		err := fmt.Errorf("failed to get response from %v, with params %v, error %v, ", endpoint, params, err)
		return *new(T), err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return *new(T), fmt.Errorf("server responded with status code: %d", response.StatusCode)
	}

	var result T
	err = json.NewDecoder(response.Body).Decode(&result)
	if err != nil {
		return *new(T), fmt.Errorf("failed to unmarshal response: %v, error: %v", response, err)
	}

	return result, nil
}

// NewJSONRPCRequest serve as a generic function for issuing any jsonrpc request and returning a T type of response
func NewJSONRPCRequest[T any](httpClient *http.Client, endpoint, method string, params any, authToken string) (T, error) {
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return *new(T), fmt.Errorf("failed to marshal params, error: %v", err)
	}

	request := jsonrpc2.Request{
		Method: method,
		Params: (*json.RawMessage)(&paramsBytes),
	}

	response, err := NewPOSTRequest[jsonrpc2.Response](httpClient, endpoint, request, authToken)

	if err != nil {
		return *new(T), err
	}

	if response.Result == nil && response.Error == nil {
		return *new(T), fmt.Errorf("invalid JSON-RPC response: both result and error fields are missing, response: %v", response)
	}

	if response.Error != nil {
		return *new(T), fmt.Errorf("JSON-RPC response error: %s", response.Error.Error())
	}

	var result T
	err = json.Unmarshal(*response.Result, &result)
	if err != nil {
		return *new(T), fmt.Errorf("failed to unmarshal result: %v, error: %v", string(*response.Result), err)
	}
	return result, nil
}
