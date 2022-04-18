package types

import (
	"encoding/json"
	"fmt"
)

// RelayType describes enum for existing relay types
type RelayType int

// Available relay types
const (
	GenericRelay RelayType = iota
	BackboneRelay
	EdgeRelay
)

var relayTypeString = [...]string{"", "backbone", "edge"}

// String returns a string representation of relay type
func (r RelayType) String() string { return relayTypeString[r] }

// UnmarshalJSON implements Unmarshaler and scans string into relay type
func (r *RelayType) UnmarshalJSON(data []byte) error {
	var v string
	err := json.Unmarshal(data, &v)
	if err != nil {
		return err
	}

	*r, err = RelayTypeFromString(v)
	return err
}

// MarshalJSON implements Marshaler and produces a string representation of relay
func (r RelayType) MarshalJSON() ([]byte, error) {
	// marshal expects quoted strings in output
	return []byte(fmt.Sprintf("%q", r.String())), nil
}

// RelayTypeFromString returns relay type from string
func RelayTypeFromString(s string) (RelayType, error) {
	for i, rt := range relayTypeString {
		if rt == s {
			return RelayType(i), nil
		}
	}

	return -1, fmt.Errorf("illegal relay type string %v", s)
}
