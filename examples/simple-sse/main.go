package main

import (
	"log"
	"net/http"
	"time"

	"go.lsl.digital/lardwaz/routing"
)

func main() {
	opts := &routing.Options{
		EnableSSE: true,
	}

	res := &routing.Resource{
		Alias:    "dummycacher",
		Method:   http.MethodGet,
		Interval: 10 * time.Second,
		URL:      "http://worldclockapi.com/api/json/est/now",
	}

	rc := routing.NewResourceCacher(opts)

	go rc.AddResource(res)

	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.Handle("/resources/sse/", rc.SSEHTTPHandler())

	log.Println("listening on http://localhost:3000")

	if err := http.ListenAndServe(":3000", nil); err != nil {
		log.Fatalf("failed to listen on :3000 :%v", err)
	}
}
