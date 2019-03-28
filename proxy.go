package routing

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

type WebAppProxy struct{ url *url.URL }

func NewWebAppProxy(url *url.URL) *WebAppProxy {
	return &WebAppProxy{url: url}
}

func (p WebAppProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var handler http.Handler
	if IsWebSocket(r) {
		handler = NewWebSocketReverseProxy(p.url)
	} else {
		handler = httputil.NewSingleHostReverseProxy(p.url)
	}

	handler.ServeHTTP(w, r)
}
