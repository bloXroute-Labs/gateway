package mock

import (
	"context"
	"crypto/md5" //nolint:gosec
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/davecgh/go-spew/spew"
)

// Hasher is a hasher function
type Hasher func(args ...any) string

// DefaultHasher is the default hasher
func DefaultHasher(args ...any) string {
	hash := md5.New() //nolint:gosec
	for _, arg := range args {
		strArg := fmt.Sprintf("%v", arg)
		hash.Write([]byte(strArg))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// Caller is a mock caller
type Caller struct {
	Hasher  Hasher
	RespMap map[string][]byte
}

// CallContext calls the given method with the given args
func (c *Caller) CallContext(_ context.Context, result any, method string, args ...any) error {
	hash := c.Hasher(append(args, method)...)
	spew.Dump(map[string]any{
		method: args,
	})
	resp, exists := c.RespMap[hash]
	if !exists {
		return errors.New("response not found")
	}

	return json.Unmarshal(resp, result)
}

// Close closes the caller
func (c *Caller) Close() {}
