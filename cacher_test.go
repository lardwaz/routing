package routing

import (
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestServeHTTP(t *testing.T) {
	when := time.Now().Format(time.RFC1123)
	numRequests := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Date", when)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))

		numRequests++
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	type test struct {
		alias          string
		method         string
		interval       time.Duration
		origin         string
		allowedOrigins []string
	}

	type result struct {
		content    []byte
		header     http.Header
		statusCode int
	}

	commonVaryHeaders := []string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"}

	tests := []struct {
		name   string
		test   test
		result result
	}{
		{
			name: "normal get",
			test: test{
				alias:    "normalget",
				method:   http.MethodGet,
				interval: time.Second,
			},
			result: result{
				content: []byte(`{"status": "ok"}`),
				header: http.Header{
					"Content-Length": []string{"16"},
					"Content-Type":   []string{"application/json"},
					"Date":           []string{when},
					"Etag":           []string{fmt.Sprintf("%x", sha1.Sum([]byte(`{"status": "ok"}`)))},
					"Cache-Control":  []string{fmt.Sprintf("max-age=%d", time.Second/time.Second)},
					"Vary":           commonVaryHeaders,
				},
				statusCode: http.StatusOK,
			},
		},
		{
			name: "good origin",
			test: test{
				alias:          "goodorigin",
				method:         http.MethodGet,
				interval:       time.Second,
				origin:         "http://good.origin",
				allowedOrigins: []string{"http://good.origin"},
			},
			result: result{
				content: []byte(`{"status": "ok"}`),
				header: http.Header{
					"Content-Length":              []string{"16"},
					"Content-Type":                []string{"application/json"},
					"Date":                        []string{when},
					"Etag":                        []string{fmt.Sprintf("%x", sha1.Sum([]byte(`{"status": "ok"}`)))},
					"Cache-Control":               []string{fmt.Sprintf("max-age=%d", time.Second/time.Second)},
					"Access-Control-Allow-Origin": []string{"http://good.origin"},
					"Vary":                        commonVaryHeaders,
				},
				statusCode: http.StatusOK,
			},
		},
	}

	for _, tt := range tests {
		numRequests = 0

		t.Run(tt.name, func(t *testing.T) {
			ts := tt.test
			rs := tt.result

			c := NewResourceCacher()
			c.AddCacheItem(ts.alias, ts.method, srv.URL+"/get", ts.interval, ts.allowedOrigins...)
			s := httptest.NewServer(c)
			defer s.Close()

			req := httptest.NewRequest(ts.method, s.URL+"/?alias="+ts.alias, nil)
			req.Header.Set("Origin", ts.origin)
			w := httptest.NewRecorder()
			c.ServeHTTP(w, req)
			r := w.Result()

			b, err := ioutil.ReadAll(r.Body)
			defer r.Body.Close()
			if err != nil {
				t.Errorf("read error: %s", err)
				return
			}

			if !reflect.DeepEqual(rs.content, b) {
				t.Errorf("<response> cache content not equal. expected %s obtained %s\n", rs.content, b)
			}

			if !reflect.DeepEqual(rs.header, r.Header) {
				t.Errorf("<response> header not equal. expected %v obtained %v\n", rs.header, r.Header)
			}

			if rs.statusCode != r.StatusCode {
				t.Errorf("<response> statusCode not equal. expected %v obtained %v\n", rs.statusCode, r.StatusCode)
			}
		})
	}
}
