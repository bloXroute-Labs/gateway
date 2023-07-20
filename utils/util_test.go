package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractArgsToMap(t *testing.T) {
	testArgString := "dummyCommand --key0 --key1 value1 --key2=value2 --key3 value3 value3 --key4"
	argsMap := ExtractArgsToMap(testArgString)

	value0, ok := argsMap["key0"]
	assert.True(t, ok)
	assert.Equal(t, value0, "")

	value1, ok := argsMap["key1"]
	assert.True(t, ok)
	assert.Equal(t, value1, "value1")

	value2, ok := argsMap["key2"]
	assert.True(t, ok)
	assert.Equal(t, value2, "value2")

	value3, ok := argsMap["key3"]
	assert.True(t, ok)
	assert.Equal(t, value3, "value3 value3")

	value4, ok := argsMap["key4"]
	assert.True(t, ok)
	assert.Equal(t, value4, "")
}
