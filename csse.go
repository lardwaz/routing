package routing

import (
	"encoding/json"
	"net/http"

	"github.com/JulesMike/go-sse"
)

const csseCommonChannel = "common"

type sseMessage struct {
	Alias   string `json:"alias"`
	Payload string `json:"payload"`
}

// CSSEResourceCacher is an SSE variant of Resource Cacher
type CSSEResourceCacher struct {
	*ResourceCacher

	server *sse.Server
}

// NewCSSEResourceCacher returns a new SSE resource cachner
func NewCSSEResourceCacher(opts *SSEOptions) *CSSEResourceCacher {
	if opts == nil {
		opts = &SSEOptions{}
	}

	c := &CSSEResourceCacher{ResourceCacher: NewResourceCacher(opts.Options)}

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
		OnClientConnect: func(client *sse.Client) {
			// Replay last messages
			for _, res := range c.resources {
				b, err := json.Marshal(sseMessage{
					Alias:   res.Alias,
					Payload: string(res.Content),
				})
				if err != nil {
					return
				}

				client.SendMessage(sse.NewMessage(res.Alias+"-"+res.Hash, string(b), "message"))
			}
		},
		ChannelNameFunc: func(r *http.Request) string {
			return csseCommonChannel
		},
		Logger: c.ResourceCacher.opts.Logger,
	})

	c.OnResourceUpdated = func(res *Resource) {
		if c.server == nil || res.OldHash == res.Hash {
			return
		}

		b, err := json.Marshal(sseMessage{
			Alias:   res.Alias,
			Payload: string(res.Content),
		})
		if err != nil {
			return
		}

		c.server.SendMessage(csseCommonChannel, sse.NewMessage(res.Alias+"-"+res.Hash, string(b), "message"))
	}

	c.OnStarted = func() {
		if c.server == nil {
			return
		}
		c.server.AddChannel(csseCommonChannel)
		c.server.Restart()
	}

	c.OnStopped = func() {
		if c.server == nil {
			return
		}

		c.server.CloseChannel(csseCommonChannel)
		c.server.Shutdown()
	}

	return c
}

func (c *CSSEResourceCacher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if c.server == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("SSE support not enabled"))
		return
	}

	for _, resource := range c.resources {
		origin := r.Header.Get("Origin")
		if !resource.IsOriginAllowed(origin) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Invalid Origin for " + resource.Alias))
			return
		}
	}

	writeCommonHeaders(w, r)

	c.server.ServeHTTP(w, r)

}
