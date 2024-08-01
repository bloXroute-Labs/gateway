package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// NewPOSTRequest servers as a generic post request handler and accepts Authorization token optionally
func NewPOSTRequest[T any](client *http.Client, endpoint string, params any, authToken string) (T, error) {
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return *new(T), fmt.Errorf("failed to marshal params, error: %v", err)
	}

	request, err := http.NewRequest("POST", endpoint, bytes.NewReader(paramsBytes))
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

	var result T
	err = json.NewDecoder(response.Body).Decode(&result)
	if err != nil {
		return *new(T), fmt.Errorf("failed to unmarshal result: %v, error: %v", response, err)
	}

	return result, nil
}
