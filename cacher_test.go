package routing_test

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"go.lsl.digital/lardwaz/routing"
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
		res      *routing.Resource
		onUpdate routing.ResourceEvent
		opts     *routing.Options
		origin   string
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
				res: &routing.Resource{
					Alias:    "normalget",
					Method:   http.MethodGet,
					Interval: time.Second,
				},
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
				res: &routing.Resource{
					Alias:          "goodorigin",
					Method:         http.MethodGet,
					Interval:       time.Second,
					AllowedOrigins: []string{"http://good.origin"},
				},
				origin: "http://good.origin",
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
		{
			name: "simple transform fn",
			test: test{
				res: &routing.Resource{
					Alias:    "simpletransformfn",
					Method:   http.MethodGet,
					Interval: time.Second,
				},
				onUpdate: func(r *routing.Resource) {
					type result struct {
						Status string `json:"status"`
					}
					var res result
					if err := json.Unmarshal(r.Content, &res); err != nil {
						return
					}

					res.Status = "transformed"

					newRes, err := json.Marshal(res)
					if err != nil {
						return
					}

					r.Content = newRes
					r.Hash = fmt.Sprintf("%x", sha1.Sum(r.Content))
					r.Header.Set("Content-Length", fmt.Sprintf("%d", len(r.Content)))
					r.Header.Set("Etag", r.Hash)
				},
			},
			result: result{
				content: []byte(`{"status":"transformed"}`),
				header: http.Header{
					"Content-Length": []string{"24"},
					"Content-Type":   []string{"application/json"},
					"Date":           []string{when},
					"Etag":           []string{fmt.Sprintf("%x", sha1.Sum([]byte(`{"status":"transformed"}`)))},
					"Cache-Control":  []string{fmt.Sprintf("max-age=%d", time.Second/time.Second)},
					"Vary":           commonVaryHeaders,
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

			c := routing.NewResourceCacher(ts.opts)
			ts.res.URL = srv.URL + "/get"
			c.AddResource(ts.res, ts.onUpdate)
			s := httptest.NewServer(c)
			defer s.Close()

			req := httptest.NewRequest(ts.res.Method, s.URL+"/?alias="+ts.res.Alias, nil)
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
