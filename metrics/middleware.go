package metrics

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

type contextKey struct{}

var (
	requestIDCtxKey = contextKey{}
)

// WrapWithRequestID adds a request id into context
func WrapWithRequestID(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.NewString()
		span, _ := tracer.SpanFromContext(r.Context())
		span.SetTag(RequestID, requestID)

		r = r.WithContext(context.WithValue(r.Context(), requestIDCtxKey, requestID))
		h.ServeHTTP(w, r)
	}
}

// GetRequestID returns request id from request context
func GetRequestID(ctx context.Context) string {
	requestID := ctx.Value(requestIDCtxKey)
	if requestID == nil {
		return ""

	}
	val, _ := requestID.(string)
	return val
}
