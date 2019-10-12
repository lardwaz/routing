package routing

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/alexandrevicenzi/go-sse"
)

// TransformFn takes a cache content and transforms it
type TransformFn func(in []byte) (out []byte)

// Resource represents a single resource to cache
type Resource struct {
	Alias          string
	Method         string
	URL            string
	Interval       time.Duration
	Content        []byte
	Header         http.Header
	StatusCode     int
	Hash           string
	AllowedOrigins []string
	TransformFn    TransformFn

	running     bool
	stopFetcher chan (struct{})
	rc          *ResourceCacher
	mu          sync.Mutex
}

// Fetch makes the request to obtain the resource and caches the result
func (r *Resource) Fetch() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cli := &http.Client{
		Timeout: time.Second * 10,
	}

	req, err := http.NewRequest(r.Method, r.URL, nil)
	if err != nil {
		return err
	}

	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if r.TransformFn != nil {
		b = r.TransformFn(b)
		resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(b)))
	}

	hash := fmt.Sprintf("%x", sha1.Sum(b))

	// if content is not new, we skip
	if r.Hash == hash {
		return nil
	}

	r.Hash = hash
	r.Content = b
	r.StatusCode = resp.StatusCode
	r.Header = resp.Header.Clone()

	// Cache control headers
	r.Header.Set("Etag", r.Hash)
	r.Header.Set("Cache-Control", fmt.Sprintf("max-age=%d", r.Interval/time.Second))

	// Inform clients on this resource SSE channel
	if r.rc.opts.EnableSSE && r.rc.sseServer != nil {
		r.rc.sseServer.SendMessage(r.Alias, sse.SimpleMessage(string(b)))
	}

	return nil
}

// IsOriginAllowed checks if origin is valid
func (r *Resource) IsOriginAllowed(origin string) bool {
	if !r.isOriginCheckEnabled() {
		return true
	}

	// No need to go any further
	if origin == "" {
		return false
	}

	for _, o := range r.AllowedOrigins {
		if o == origin {
			return true
		}
	}

	return false
}

func (r *Resource) isOriginCheckEnabled() bool {
	// Check if origin check enabled
	return r.AllowedOrigins != nil && len(r.AllowedOrigins) != 0
}

// StartFetcher starts the automatic fetcher
func (r *Resource) StartFetcher() {
	if r.running {
		// Already running
		return
	}

	r.running = true
	ticker := time.NewTicker(r.Interval)
	r.Fetch()
	go func() {
		for {
			select {
			case <-ticker.C:
				r.Fetch()
			case <-r.stopFetcher:
				r.running = false
				return
			}
		}
	}()
}

// StopFetcher stops the automatic fetcher
func (r *Resource) StopFetcher() {
	r.stopFetcher <- struct{}{}
}

// WriteHeaders write the header to a response writer
func (r *Resource) WriteHeaders(w http.ResponseWriter) {
	for k, v := range r.Header {
		for _, v2 := range v {
			w.Header().Set(k, v2)
		}
	}
}

// Options represents a set of resource cacher options
type Options struct {
	EnableSSE bool
	// RetryInterval change EventSource default retry interval (milliseconds)
	SSERetryInterval int
	// Defines a custom logger
	Logger *log.Logger
}

// ResourceCacher creates a reverse proxy that caches the results
type ResourceCacher struct {
	resources map[string]*Resource
	sseServer *sse.Server
	opts      *Options
}

// NewResourceCacher creates a new resource cacher
func NewResourceCacher(opts *Options) *ResourceCacher {
	rc := &ResourceCacher{
		resources: make(map[string]*Resource),
		opts:      opts,
	}

	if rc.opts == nil {
		rc.opts = &Options{}
	}

	if rc.opts.Logger == nil {
		rc.opts.Logger = log.New(os.Stdout, "cacher: ", log.Ldate|log.Ltime|log.Lshortfile)
	}

	if rc.opts.EnableSSE {
		// Increase default retry interval to 5s
		if rc.opts.SSERetryInterval == 0 {
			rc.opts.SSERetryInterval = 5 * 1000
		}

		rc.sseServer = sse.NewServer(&sse.Options{
			RetryInterval: rc.opts.SSERetryInterval,
			ChannelNameFunc: func(r *http.Request) string {
				// Use alias query in url as channel name
				alias, err := getAliasFromRequest(r)
				if err != nil {
					return r.URL.Path
				}

				return alias
			},
			Logger: rc.opts.Logger,
		})
	}

	return rc
}

// AddResource adds a new cache item to the resource cacher
func (c *ResourceCacher) AddResource(res *Resource) (*Resource, error) {
	if res.Alias == "" {
		return nil, errors.New("missing alias")
	}

	if res.Method == "" {
		return nil, errors.New("missing method")
	}

	if res.URL == "" {
		return nil, errors.New("missing url")
	}

	if res.Interval == 0 {
		return nil, errors.New("invalid interval")
	}

	res.rc = c

	res.StartFetcher()

	c.resources[res.Alias] = res

	return res, nil
}

// Start autofetching/caching
func (c *ResourceCacher) Start() {
	for _, resource := range c.resources {
		resource.StartFetcher()
	}
}

// Stop autofetching/caching
func (c *ResourceCacher) Stop() {
	for _, resource := range c.resources {
		resource.StopFetcher()
	}

	if c.sseServer != nil {
		c.sseServer.Shutdown()
	}
}

// SSEHTTPHandler returns the sse http handler
func (c *ResourceCacher) SSEHTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !c.opts.EnableSSE {
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

		resource.WriteHeaders(w)

		c.sseServer.ServeHTTP(w, r)
	})
}

// ServeHTTP to implement net/http.Handler for ResourceCacher
func (c *ResourceCacher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	if match := r.Header.Get("If-None-Match"); match != "" {
		if resource.Hash == match {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	writeCommonHeaders(w, r)

	resource.WriteHeaders(w)

	w.WriteHeader(resource.StatusCode)
	w.Write(resource.Content)
}

func writeCommonHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Origin")
	w.Header().Add("Vary", "Access-Control-Request-Method")
	w.Header().Add("Vary", "Access-Control-Request-Headers")
	if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
}

func getAliasFromRequest(r *http.Request) (string, error) {
	query := r.URL.Query()

	aliases, ok := query["alias"]
	if !ok {
		return "", errors.New("Missing alias")
	}

	return aliases[0], nil
}
