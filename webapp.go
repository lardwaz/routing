package routing

import (
	"net/http"
)

//ErrorHandler defines a custom error handler
type ErrorHandler func(w http.ResponseWriter, status int)

//WrapWithErrorHandler wraps an http.Handler function with a custom error handling func
func WrapWithErrorHandler(next http.Handler, errorHandler ErrorHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w = &responseWriter{w, errorHandler, false}
		next.ServeHTTP(w, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	errorHandler ErrorHandler
	failed       bool
}

func (w *responseWriter) Write(p []byte) (n int, err error) {
	if w.failed {
		return len(p), nil
	}

	return w.ResponseWriter.Write(p)
}

func (w *responseWriter) WriteHeader(status int) {
	if status < http.StatusBadRequest {
		w.ResponseWriter.WriteHeader(status)
	} else if w.errorHandler != nil {
		w.failed = true
		w.errorHandler(w.ResponseWriter, status)
		w.errorHandler = nil
	}
}

//WrapWithFallback wraps an http.Handler function in order to show fallback content on error
func WrapWithFallback(handler http.Handler, fallback []byte) http.Handler {
	return WrapWithErrorHandler(handler, ErrorHandler(func(w http.ResponseWriter, status int) {
		w.WriteHeader(http.StatusOK)
		w.Write(fallback)
	}))
}
