package routing

import (
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
		alias    string
		method   string
		url      string
		interval time.Duration
	}

	type result struct {
		cache      []byte
		header     http.Header
		statusCode int
	}

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
				url:      "http://localhost:9999/get?alias=normalget",
				interval: time.Second,
			},
			result: result{
				cache: []byte(`{"status": "ok"}`),
				header: http.Header{
					"Content-Length": []string{"16"},
					"Content-Type":   []string{"application/json"},
					"Date":           []string{when},
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
			cache := c.AddCacheItem(ts.alias, ts.method, srv.URL+"/get", ts.interval)
			s := httptest.NewServer(c)
			defer s.Close()

			req := httptest.NewRequest(ts.method, s.URL+"/?alias="+ts.alias, nil)
			w := httptest.NewRecorder()
			c.ServeHTTP(w, req)
			r := w.Result()

			b, err := ioutil.ReadAll(r.Body)
			defer r.Body.Close()
			if err != nil {
				t.Errorf("read error: %s", err)
				return
			}

			if !reflect.DeepEqual(rs.cache, cache.Content) {
				t.Errorf("cache not equal. expected %s obtained %s\n", rs.cache, b)
			} else if !reflect.DeepEqual(rs.cache, b) {
				t.Errorf("<response> cache not equal. expected %s obtained %s\n", rs.cache, b)
			}

			// if !reflect.DeepEqual(rs.header, c.header) {
			// 	t.Errorf("header not equal. expected %v obtained %v\n", rs.header, r.Header)
			// } else if !reflect.DeepEqual(rs.header, r.Header) {
			// 	t.Errorf("<response> header not equal. expected %v obtained %v\n", rs.header, r.Header)
			// }

			if rs.statusCode != cache.StatusCode {
				t.Errorf("statusCode not equal. expected %v obtained %v\n", rs.statusCode, r.StatusCode)
			} else if rs.statusCode != r.StatusCode {
				t.Errorf("<response> statusCode not equal. expected %v obtained %v\n", rs.statusCode, r.StatusCode)
			}
		})
	}
}