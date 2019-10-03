package routing

import (
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

// CacheItem represents a single resource to cache
type CacheItem struct {
	Alias         string
	Method        string
	URL           string
	Content       []byte
	ContentLength int64
	Header        http.Header
	StatusCode    int
	Hash          string

	interval time.Duration
	running  bool
	stop     chan struct{}
	lock     sync.Mutex
}

// Fetch makes the request to obtain the resource and caches the result
func (c *CacheItem) Fetch() error {
	c.lock.Lock()
	defer c.lock.Unlock()

	cli := &http.Client{
		Timeout: time.Second * 10,
	}

	req, err := http.NewRequest(c.Method, c.URL, nil)
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

	c.Content = b
	c.ContentLength = resp.ContentLength
	c.Header = resp.Header.Clone()
	c.StatusCode = resp.StatusCode
	c.Hash = fmt.Sprintf("%x", sha1.Sum(b))

	return nil
}

// StartFetcher starts the automatic fetcher
func (c *CacheItem) StartFetcher() {
	if c.running {
		// Already running
		return
	}

	c.running = true
	ticker := time.NewTicker(c.interval)
	c.Fetch()
	go func() {
		select {
		case <-ticker.C:
			c.Fetch()
		case <-c.stop:
			c.running = false
			return
		}
	}()
}

// StopFetcher stops the automatic fetcher
func (c *CacheItem) StopFetcher() {
	c.stop <- struct{}{}
}

// ResourceCacher creates a reverse proxy that caches the results
type ResourceCacher struct {
	stop  chan struct{}
	cache map[string]*CacheItem
}

// NewResourceCacher creates a new resource cacher
func NewResourceCacher() *ResourceCacher {
	return &ResourceCacher{
		cache: make(map[string]*CacheItem),
		stop:  make(chan struct{}),
	}
}

// AddCacheItem adds a new cache item to the resource cacher
func (c *ResourceCacher) AddCacheItem(alias, method, url string, interval time.Duration) *CacheItem {
	cache := &CacheItem{
		Alias:    alias,
		Method:   method,
		URL:      url,
		interval: interval,
	}

	cache.StartFetcher()

	c.cache[alias] = cache

	return cache
}

// Start autofetching/caching
func (c *ResourceCacher) Start() {
	for _, cacheItem := range c.cache {
		cacheItem.StartFetcher()
	}
}

// Stop autofetching/caching
func (c *ResourceCacher) Stop() {
	for _, cacheItem := range c.cache {
		cacheItem.StopFetcher()
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

	cache, ok := c.cache[alias]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid alias"))
		return
	}

	if match := r.Header.Get("If-None-Match"); match != "" {
		if cache.Hash == match {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	for k, v := range cache.Header {
		for _, v2 := range v {
			w.Header().Set(k, v2)
		}
	}
	w.Header().Set("Etag", cache.Hash)
	w.WriteHeader(cache.StatusCode)
	w.Write(cache.Content)
}
