# ratelim

`ratelim` is a Go module for managing rate limiters, particularly for use by an `http.Client` within a
custom `http.RoundTripper` to rate limit outgoing requests.

## Usage
A common use case is to create an `http.RoundTripper` which will apply a rate limiter per target origin:

```golang
package main

import (
"fmt"
"log"
"net/http"
"sync"

"github.com/milo-minderbinder/ratelim"
)

func main() {
	// Create a client with a 10.0 request/second rate limit per origin, with a 2 request burst limit:
	client := http.Client{
		Transport: ratelim.PerOriginRoundTripper(10.0, 2, http.DefaultTransport),
	}

	reqs := make([]*http.Request, 5)
	for i := 0; i < 5; i++ {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://example.com/%d", i), nil)
		if err != nil {
			log.Fatalln(err)
		}
		reqs = append(reqs, req)
	}

	// Start sending all requests concurrently but cap the actual request rate according to the configured rate limiter:
	var wg sync.WaitGroup
	for i := range reqs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := client.Do(reqs[i])
			if err != nil {
				fmt.Println(err)
			}
			fmt.Printf("%s %s\n", resp.Status, resp.Request.URL.String())
		}(i)
	}
	wg.Wait()
}
```