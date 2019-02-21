package routing

import (
	"net/http"

	"github.com/gorilla/mux"
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

//AddWebAppRouting creates routing for webapp. It attempts to serve from filesystem; with custom fallback content upon failure
func AddWebAppRouting(router *mux.Router, prefix string, fs http.FileSystem, fallback []byte, mwf ...mux.MiddlewareFunc) error {
	subRoute := router.PathPrefix(prefix).Subrouter()
	errorHandler := ErrorHandler(func(w http.ResponseWriter, status int) {
		w.WriteHeader(http.StatusOK)
		w.Write(fallback)
	})

	for _, m := range mwf {
		subRoute.Use(m)
	}

	subRoute.PathPrefix("/").Handler(
		http.StripPrefix(
			prefix,
			WrapWithErrorHandler(http.FileServer(fs), errorHandler),
		),
	)

	return nil

}
