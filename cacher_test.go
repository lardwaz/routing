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
				method:   http.MethodGet,
				url:      "http://localhost:9999/get",
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

	for i, tt := range tests {
		numRequests = 0
		runner := func(i int, ts test, rs result) {

			c := NewResourceCacher(ts.method, srv.URL+"/get", ts.interval)
			s := httptest.NewServer(c)
			defer s.Close()

			req := httptest.NewRequest(ts.method, s.URL, nil)
			w := httptest.NewRecorder()
			c.ServeHTTP(w, req)
			r := w.Result()

			b, err := ioutil.ReadAll(r.Body)
			defer r.Body.Close()
			if err != nil {
				t.Errorf("[%d] read error: %s", i, err)
				return
			}

			if !reflect.DeepEqual(rs.cache, c.cache) {
				t.Errorf("[%d] cache not equal. expected %s obtained %s\n", i, rs.cache, b)
			} else if !reflect.DeepEqual(rs.cache, b) {
				t.Errorf("[%d] <response> cache not equal. expected %s obtained %s\n", i, rs.cache, b)
			}

			// if !reflect.DeepEqual(rs.header, c.header) {
			// 	t.Errorf("[%d] header not equal. expected %v obtained %v\n", i, rs.header, r.Header)
			// } else if !reflect.DeepEqual(rs.header, r.Header) {
			// 	t.Errorf("[%d] <response> header not equal. expected %v obtained %v\n", i, rs.header, r.Header)
			// }

			if rs.statusCode != c.statusCode {
				t.Errorf("[%d] statusCode not equal. expected %v obtained %v\n", i, rs.statusCode, r.StatusCode)
			} else if rs.statusCode != r.StatusCode {
				t.Errorf("[%d] <response> statusCode not equal. expected %v obtained %v\n", i, rs.statusCode, r.StatusCode)
			}
		}

		runner(i, tt.test, tt.result)
		runner(i, tt.test, tt.result)
	}
}
