package routing

import (
	"fmt"
	"net/http"

	"github.com/JulesMike/go-sse"
)

// SSEOptions augments Resource Cacher Options
type SSEOptions struct {
	*Options

	RetryInterval int
}

// SSEResourceCacher is an SSE variant of Resource Cacher
type SSEResourceCacher struct {
	*ResourceCacher

	server *sse.Server
}

// NewSSEResourceCacher returns a new SSE resource cachner
func NewSSEResourceCacher(opts *SSEOptions) *SSEResourceCacher {
	if opts == nil {
		opts = &SSEOptions{}
	}

	c := &SSEResourceCacher{ResourceCacher: NewResourceCacher(opts.Options)}

	// Increase default retry interval to 5s
	if opts.RetryInterval == 0 {
		opts.RetryInterval = 5 * 1000
	}

	// Create new SSE Server
	c.server = sse.NewServer(&sse.Options{
		RetryInterval: opts.RetryInterval,
		Headers: map[string]string{
			"Access-Control-Allow-Methods": "GET, OPTIONS",
			"Access-Control-Allow-Headers": "Keep-Alive,X-Requested-With,Cache-Control,Content-Type,Last-Event-ID",
		},
		ChannelNameFunc: func(r *http.Request) string {
			// Use alias query in url as channel name
			alias, err := getAliasFromRequest(r)
			if err != nil {
				return r.URL.Path
			}

			return alias
		},
		Logger: c.ResourceCacher.opts.Logger,
	})

	c.OnResourceAdded = func(res *Resource) {
		if c.server == nil || c.server.HasChannel(res.Alias) {
			return
		}

		c.server.AddChannel(res.Alias)
	}

	c.OnResourceUpdated = func(res *Resource) {
		if c.server == nil || !c.server.HasChannel(res.Alias) {
			return
		}

		c.server.SendMessage(res.Alias, sse.NewMessage(res.Hash, string(res.Content), "message"))
	}

	c.OnResourceRemoved = func(res *Resource) {
		if c.server == nil || !c.server.HasChannel(res.Alias) {
			return
		}

		c.server.CloseChannel(res.Alias)
	}

	c.OnStarted = func() {
		if c.server == nil {
			return
		}

		c.server.Restart()
	}

	c.OnStopped = func() {
		if c.server == nil {
			return
		}

		c.server.Shutdown()
	}

	return c
}

func (c *SSEResourceCacher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if c.server == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("SSE support not enabled"))
		return
	}

	alias, err := getAliasFromRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf("%v", err)))
		return
	}

	resource, ok := c.resources[alias]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid alias"))
		return
	}

	origin := r.Header.Get("Origin")
	if !resource.IsOriginAllowed(origin) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Invalid Origin"))
		return
	}

	writeCommonHeaders(w, r)

	c.server.ServeHTTP(w, r)
}
