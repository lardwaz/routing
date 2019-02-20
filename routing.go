package routing // import "go.lsl.digital/gocipe/routing"

import (
	"net/http"
	"net/http/httputil" // DevRouting returns a new subrouter
	"net/url"

	"github.com/gorilla/mux"
)

//AddDev adds development routing - typically to nodejs backend (for frontend development); both via http and websocket
func AddDev(router *mux.Router, prefix string, port string, mwf ...mux.MiddlewareFunc) error {
	var (
		url    = &url.URL{Scheme: "http", Host: "localhost:" + port}
		wsprox = NewWebSocketReverseProxy(url)
		htprox = httputil.NewSingleHostReverseProxy(url)
	)

	subRoute := router.PathPrefix(prefix).Subrouter()

	for _, m := range mwf {
		subRoute.Use(m)
	}

	subRoute.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IsWebSocket(r) {
			wsprox.ServeHTTP(w, r)
		} else {
			htprox.ServeHTTP(w, r)
		}
	})

	return nil
}
