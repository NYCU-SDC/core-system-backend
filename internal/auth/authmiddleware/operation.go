package authmiddleware

import (
	"NYCU-SDC/core-system-backend/internal"
	"net/http"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

type Middleware func(http.HandlerFunc) http.HandlerFunc

type Operation struct {
	logger        *zap.Logger
	problemWriter *problem.HttpWriter
}

func NewOperation(
	logger *zap.Logger,
	problemWriter *problem.HttpWriter,
) *Operation {
	return &Operation{
		logger:        logger,
		problemWriter: problemWriter,
	}
}

func (o *Operation) And(middlewares ...Middleware) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {

		handler := next

		for i := len(middlewares) - 1; i >= 0; i-- {
			handler = middlewares[i](handler)
		}

		return handler
	}
}

func (o *Operation) Or(middlewares ...Middleware) Middleware {
	tracer := otel.Tracer("auth/middleware")
	return func(next http.HandlerFunc) http.HandlerFunc {

		return func(w http.ResponseWriter, r *http.Request) {

			traceCtx, span := tracer.Start(r.Context(), "AuthOr")
			defer span.End()
			logger := logutil.WithContext(traceCtx, o.logger)

			for _, m := range middlewares {

				recorder := newResponseRecorder()

				calledNext := false

				m(func(w http.ResponseWriter, r *http.Request) {
					calledNext = true
					next(w, r)
				})(recorder, r.WithContext(traceCtx))

				if calledNext {
					return
				}
			}

			logger.Warn("permission denied",
				zap.String("path", r.URL.Path),
				zap.String("method", r.Method),
			)

			o.problemWriter.WriteError(traceCtx, w, internal.ErrPermissionDenied, logger)
		}
	}
}

type responseRecorder struct {
	header http.Header
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{
		header: make(http.Header),
	}
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return len(b), nil
}

func (r *responseRecorder) WriteHeader(statusCode int) {}
