package routing

import (
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

// Resource represents a single resource to cache
type Resource struct {
	Alias          string
	Method         string
	URL            string
	Interval       time.Duration
	Content        []byte
	ContentLength  int64
	Header         http.Header
	StatusCode     int
	Hash           string
	AllowedOrigins []string

	running bool
	stop    chan struct{}
	lock    sync.Mutex
}

// Fetch makes the request to obtain the resource and caches the result
func (r *Resource) Fetch() error {
	r.lock.Lock()
	defer r.lock.Unlock()

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

	r.Content = b
	r.ContentLength = resp.ContentLength
	r.StatusCode = resp.StatusCode
	r.Hash = fmt.Sprintf("%x", sha1.Sum(b))
	r.Header = resp.Header.Clone()

	// Caching stuffs
	r.Header.Set("Etag", r.Hash)
	r.Header.Set("Cache-Control", fmt.Sprintf("max-age=%d", r.Interval/time.Second))

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
			case <-r.stop:
				r.running = false
				return
			}
		}
	}()
}

// StopFetcher stops the automatic fetcher
func (r *Resource) StopFetcher() {
	r.stop <- struct{}{}
}

// ResourceCacher creates a reverse proxy that caches the results
type ResourceCacher struct {
	resources map[string]*Resource
}

// NewResourceCacher creates a new resource cacher
func NewResourceCacher() *ResourceCacher {
	return &ResourceCacher{
		resources: make(map[string]*Resource),
	}
}

// AddResource adds a new cache item to the resource cacher
func (c *ResourceCacher) AddResource(alias, method, url string, interval time.Duration, allowedOrigins ...string) *Resource {
	cache := &Resource{
		Alias:          alias,
		Method:         method,
		URL:            url,
		Interval:       interval,
		AllowedOrigins: allowedOrigins,
	}

	cache.StartFetcher()

	c.resources[alias] = cache

	return cache
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
}

// ServeHTTP to implement net/http.Handler for ResourceCacher
func (c *ResourceCacher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Get alias from url
	aliases, ok := query["alias"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Missing alias"))
		return
	}
	alias := aliases[0]

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

	for k, v := range resource.Header {
		for _, v2 := range v {
			w.Header().Set(k, v2)
		}
	}

	w.Header().Add("Vary", "Origin")
	w.Header().Add("Vary", "Access-Control-Request-Method")
	w.Header().Add("Vary", "Access-Control-Request-Headers")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.WriteHeader(resource.StatusCode)
	w.Write(resource.Content)
}
