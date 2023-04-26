package ratelim

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func durationBetween(min, max time.Duration) time.Duration {
	return time.Duration(rand.Int63n(int64(max-min)+1)) + min
}

func TestPerOriginRoundTripper(t *testing.T) {
	tests := []struct {
		name        string
		minRespTime time.Duration
		maxRespTime time.Duration
		limit       rate.Limit
		burst       int
		timeout     time.Duration
		numReqs     int
	}{
		{
			name:        "no limit",
			minRespTime: 500 * time.Millisecond,
			maxRespTime: 500 * time.Millisecond,
			limit:       rate.Inf,
			burst:       0,
			timeout:     0,
			numReqs:     10,
		},
		{
			name:        "partial completion",
			minRespTime: 500 * time.Millisecond,
			maxRespTime: 500 * time.Millisecond,
			limit:       5.0,
			burst:       1,
			timeout:     1 * time.Second,
			numReqs:     5,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				ts := httptest.NewServer(
					http.HandlerFunc(
						func(w http.ResponseWriter, r *http.Request) {
							time.Sleep(durationBetween(tt.minRespTime, tt.maxRespTime))
							if err := r.Write(w); err != nil {
								_, _ = fmt.Fprintln(w, err)
							}
						},
					),
				)
				defer ts.Close()
				transport := PerOriginRoundTripper(
					tt.limit, tt.burst, nil,
				)
				// transport.Logger = log.Default()
				client := ts.Client()
				client.Transport = transport

				var ctx context.Context
				var cancel context.CancelFunc
				if tt.timeout != 0 {
					ctx, cancel = context.WithTimeout(context.Background(), tt.timeout)
				} else {
					ctx, cancel = context.WithCancel(context.Background())
				}
				defer func() { fmt.Println("ctx.Err():", ctx.Err()) }()
				defer cancel()
				reqs := make([]*http.Request, tt.numReqs)
				for i := 0; i < len(reqs); i++ {
					req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%d", ts.URL, i+1), nil)
					if err != nil {
						t.Fatal("setup failed:", err)
					}
					reqs[i] = req
				}

				type indResp struct {
					i    int
					resp *http.Response
				}
				c := make(chan *indResp, cap(reqs))
				var wg sync.WaitGroup
				start := time.Now()
				for i := range reqs {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						resp, err := client.Do(reqs[i])
						if err != nil {
							t.Log(i, err)
							cancel()
							return
						}
						c <- &indResp{i, resp}
					}(i)
				}
				wg.Wait()
				elapsed := time.Since(start).Seconds()
				close(c)
				resps := make(map[int]*http.Response, 0)
				for v := range c {
					resps[v.i] = v.resp
				}
				t.Logf(
					"finished %d/%d requests in %f seconds @ %f r/s",
					len(resps),
					len(reqs),
					elapsed,
					float64(len(resps))/elapsed,
				)
			},
		)
	}
}
