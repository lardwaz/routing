package routing

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

// WebAppProxy creates a reverse proxy typically used for nodejs webapps
type WebAppProxy struct{ url *url.URL }

// NewWebAppProxy creates a new webapp proxy
func NewWebAppProxy(url *url.URL) *WebAppProxy {
	return &WebAppProxy{url: url}
}

// ServeHTTP to implement net/http.Handler for WebAppProxy
func (p WebAppProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var handler http.Handler
	if IsWebSocket(r) {
		handler = NewWebSocketReverseProxy(p.url)
	} else {
		handler = httputil.NewSingleHostReverseProxy(p.url)
	}

	handler.ServeHTTP(w, r)
}
