package ws

import (
	"encoding/json"
	"io"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"
)

type some struct {
	Num   int       `json:"num,omitempty"`
	Str   string    `json:"str,omitempty"`
	Other someOther `json:"other,omitempty,omitzero"`
}

type someOther struct {
	Num int    `json:"num,omitempty"`
	Str string `json:"str,omitempty"`
}

func (s someOther) IsZero() bool {
	return s.Num == 0 && s.Str == ""
}

func TestJsoniterOmitZeroExt(t *testing.T) {
	api := jsoniter.Config{}.Froze()
	api.RegisterExtension(&omitZeroExt{})

	s := some{Num: 1}
	b, err := api.Marshal(s)
	require.NoError(t, err)
	require.Equal(t, `{"num":1}`, string(b))
}

func BenchmarkJsoniterOmitZeroExt(b *testing.B) {
	api := jsoniter.Config{}.Froze()
	api.RegisterExtension(&omitZeroExt{})

	resp := some{
		Str: "good",
	}

	_, _ = api.Marshal(resp)

	b.ResetTimer()
	for b.Loop() {
		_, _ = api.Marshal(resp)
	}
}

func BenchmarkJsoniter(b *testing.B) {
	api := jsoniter.Config{}.Froze()

	resp := some{
		Str: "good",
	}

	_, _ = api.Marshal(resp)

	b.ResetTimer()
	for b.Loop() {
		_, _ = api.Marshal(resp)
	}
}

func BenchmarkJsoniterStream(b *testing.B) {
	api := jsoniter.Config{}.Froze()
	api.RegisterExtension(&omitZeroExt{})

	resp := some{
		Str: "good",
	}

	st := api.BorrowStream(io.Discard)
	st.WriteVal(resp)
	api.ReturnStream(st)

	b.ResetTimer()
	for b.Loop() {
		st := api.BorrowStream(io.Discard)
		st.WriteVal(resp)
		api.ReturnStream(st)
	}
}

func BenchmarkJsoniterEncoder(b *testing.B) {
	api := jsoniter.Config{}.Froze()
	api.RegisterExtension(&omitZeroExt{})

	resp := some{
		Str: "good",
	}

	api.NewEncoder(io.Discard).Encode(resp)

	b.ResetTimer()
	for b.Loop() {
		api.NewEncoder(io.Discard).Encode(resp)
	}
}

func BenchmarkJsoniterStreamOmitEmptyExt(b *testing.B) {
	api := jsoniter.Config{}.Froze()
	api.RegisterExtension(&omitZeroExt{})

	resp := some{
		Str: "good",
	}

	st := api.BorrowStream(io.Discard)
	st.WriteVal(resp)
	api.ReturnStream(st)

	b.ResetTimer()
	for b.Loop() {
		st := api.BorrowStream(io.Discard)
		st.WriteVal(resp)
		api.ReturnStream(st)
	}
}

func BenchmarkJsoniterStd(b *testing.B) {
	resp := some{
		Str: "good",
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = json.Marshal(resp)
	}
}

func BenchmarkJsoniterStdEncoder(b *testing.B) {
	resp := some{
		Str: "good",
	}

	b.ResetTimer()
	for b.Loop() {
		json.NewEncoder(io.Discard).Encode(resp)
	}
}
