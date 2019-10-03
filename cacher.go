package routing

import (
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

// ResourceCacher creates a reverse proxy that caches the results
type ResourceCacher struct {
	url           string
	method        string
	interval      time.Duration
	cache         []byte
	contentLength int64
	header        http.Header
	statusCode    int
	lock          sync.Mutex
	active        bool
}

// NewResourceCacher creates a new resource cacher
func NewResourceCacher(method, url string, interval time.Duration) *ResourceCacher {
	return &ResourceCacher{url: url, method: method, interval: interval}
}

// Fetch makes the request to obtain the resource and caches the result
func (c *ResourceCacher) Fetch() error {
	c.lock.Lock()
	defer c.lock.Unlock()

	cli := &http.Client{
		Timeout: time.Second * 10,
	}

	req, err := http.NewRequest(c.method, c.url, nil)
	if err != nil {
		return err
	}

	resp, err := cli.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err == nil {
		c.cache = b
		c.contentLength = resp.ContentLength
		c.header = resp.Header.Clone()
		c.statusCode = resp.StatusCode
	}

	return err
}

// Start autofetching/caching
func (c *ResourceCacher) Start() {
	c.active = true
	for c.active {
		c.Fetch()
		time.Sleep(c.interval)
	}
}

// Stop autofetching/caching
func (c *ResourceCacher) Stop() {
	c.active = false
}

// ServeHTTP to implement net/http.Handler for ResourceCacher
func (c *ResourceCacher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if c.cache == nil {
		err := c.Fetch()
		if err != nil {
			for name, hdrs := range c.header {
				for i := range hdrs {
					w.Header().Set(name, hdrs[i])
				}
			}
			w.WriteHeader(http.StatusBadGateway)
		}
	}

	w.WriteHeader(c.statusCode)
	w.Write(c.cache)
}
