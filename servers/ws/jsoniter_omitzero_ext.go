package ws

import (
	"reflect"
	"strings"
	"unsafe"

	jsoniter "github.com/json-iterator/go"
)

func init() {
	jsoniter.ConfigCompatibleWithStandardLibrary.RegisterExtension(&omitZeroExt{})
}

// interface any “omitzero” type must implement
type zeroable interface {
	IsZero() bool
}

type omitZeroExt struct {
	jsoniter.DummyExtension
}

func (ext *omitZeroExt) UpdateStructDescriptor(sd *jsoniter.StructDescriptor) {
	zeroIfc := reflect.TypeOf((*zeroable)(nil)).Elem()
	for _, binding := range sd.Fields {
		tag := binding.Field.Tag().Get("json")
		if !strings.Contains(tag, "omitzero") {
			continue
		}
		// real Go type of the field
		goT := binding.Field.Type().Type1()
		if !(goT.Implements(zeroIfc) || reflect.PointerTo(goT).Implements(zeroIfc)) {
			continue
		}
		// wrap the *field* encoder for this specific struct‐field
		orig := binding.Encoder
		binding.Encoder = &fieldZeroWrapper{orig, goT}
	}
}

// this wrapper is only ever used for one field in one struct
type fieldZeroWrapper struct {
	orig   jsoniter.ValEncoder
	goType reflect.Type
}

func (w *fieldZeroWrapper) Encode(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	w.orig.Encode(ptr, stream)
}

func (w *fieldZeroWrapper) IsEmpty(ptr unsafe.Pointer) bool {
	v := reflect.NewAt(w.goType, ptr).Elem().Interface()
	return v.(zeroable).IsZero()
}
