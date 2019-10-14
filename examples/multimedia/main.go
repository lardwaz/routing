package main

import (
	"log"
	"net/http"
	"time"

	"go.lsl.digital/lardwaz/routing"
)

func main() {
	res1 := &routing.Resource{
		Alias:    "image1",
		Method:   http.MethodGet,
		Interval: 10 * time.Second,
		URL:      "https://dummyimage.com/320x240/efefef/1b1b1b.png&text=This is cached",
	}

	res2 := &routing.Resource{
		Alias:    "audio1",
		Method:   http.MethodGet,
		Interval: 10 * time.Second,
		URL:      "https://upload.wikimedia.org/wikipedia/commons/f/f2/Median_test.ogg",
	}

	res3 := &routing.Resource{
		Alias:    "video1",
		Method:   http.MethodGet,
		Interval: 10 * time.Second,
		URL:      "http://clips.vorwaerts-gmbh.de/big_buck_bunny.mp4",
	}

	res4 := &routing.Resource{
		Alias:    "text1",
		Method:   http.MethodGet,
		Interval: 10 * time.Second,
		URL:      "http://worldclockapi.com/api/json/est/now",
	}

	rc := routing.NewResourceCacher(nil)

	go rc.AddResource(res1, nil)
	go rc.AddResource(res2, nil)
	go rc.AddResource(res3, nil)
	go rc.AddResource(res4, nil)

	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.Handle("/resources/", rc)

	log.Println("listening on http://localhost:3000")

	if err := http.ListenAndServe(":3000", nil); err != nil {
		log.Fatalf("failed to listen on :3000 :%v", err)
	}
}
