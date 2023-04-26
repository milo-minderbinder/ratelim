package ratelim

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/milo-minderbinder/ratelim/syncmap"
)

func Origin(url *url.URL) string {
	// apply normalization steps which are missing/incomplete in url.URL
	// (ref: https://www.rfc-editor.org/rfc/rfc9110#name-uri-origin)
	// first, while scheme is automatically lower-cased, host(name) is not
	host := strings.ToLower(url.Hostname())
	// next, strip any leading zeros from port number
	port := strings.TrimLeft(url.Port(), "0")
	if port == "" {
		return fmt.Sprintf("%s://%s", url.Scheme, host)
	}
	if defaultPort, ok := map[string]string{
		"http":  "80",
		"https": "443",
	}[url.Scheme]; ok && port == defaultPort {
		return fmt.Sprintf("%s://%s", url.Scheme, host)
	}
	return fmt.Sprintf("%s://%s:%s", url.Scheme, host, port)
}

func TargetOrigin(r *http.Request) string {
	return Origin(r.URL)
}

func defaultTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

type Map[K comparable] struct {
	*syncmap.SyncMap[K, *rate.Limiter]
}

func NewMap[K comparable]() *Map[K] {
	return &Map[K]{
		SyncMap: syncmap.New[K, *rate.Limiter](),
	}
}

// A PerKeyRoundTripper rate limits each request sent through RoundTrip. Requests are grouped by Key and mapped to a
// rate.Limiter. If no rate.Limiter exists for a given Key value yet, one is instantiated with the default rate.Limit
// and burst as returned by LimiterDefaults.
//
// In this way, requests can be rate limited per host, for example, or whatever grouping makes sense for
// a given use case.
type PerKeyRoundTripper[K comparable] struct {
	defaultLimit rate.Limit
	defaultBurst int
	keyFunc      func(*http.Request) K
	limiters     *Map[K]
	mux          sync.RWMutex
	http.RoundTripper
	Logger *log.Logger
}

// NewPerKeyRoundTripper creates a new PerKeyRoundTripper. The defaultLimit and defaultBurst determine the rate.Limit
// and burst parameters used to create a new rate.Limiter when none is mapped yet to a given Key value. The keyFunc is
// the function used to derive the Key used to map any given request to a particular rate.Limiter. The roundTripper
// parameter sets the underlying http.RoundTripper used to send each request after applying the rate limiter; if nil, a
// new *http.Transport is created with defaults based on http.DefaultTransport.
func NewPerKeyRoundTripper[K comparable](
	defaultLimit rate.Limit,
	defaultBurst int,
	keyFunc func(*http.Request) K,
	roundTripper http.RoundTripper,
) *PerKeyRoundTripper[K] {
	if roundTripper == nil {
		roundTripper = defaultTransport()
	}
	return &PerKeyRoundTripper[K]{
		defaultLimit: defaultLimit,
		defaultBurst: defaultBurst,
		keyFunc:      keyFunc,
		limiters:     NewMap[K](),
		RoundTripper: roundTripper,
	}
}

func (t *PerKeyRoundTripper[K]) LimiterDefaults() (limit rate.Limit, burst int) {
	t.mux.RLock()
	defer t.mux.RUnlock()
	return t.defaultLimit, t.defaultBurst
}

func (t *PerKeyRoundTripper[K]) SetLimiterDefaults(limit rate.Limit, burst int) {
	t.mux.Lock()
	defer t.mux.Unlock()
	t.defaultLimit = limit
	t.defaultBurst = burst
}

func (t *PerKeyRoundTripper[K]) Key(req *http.Request) K {
	return t.keyFunc(req)
}

func (t *PerKeyRoundTripper[K]) Limiter(req *http.Request) *rate.Limiter {
	limiter, _ := t.limiters.LoadOrStore(t.Key(req), rate.NewLimiter(t.LimiterDefaults()))
	return limiter
}

func (t *PerKeyRoundTripper[K]) Limiters() *Map[K] {
	return t.limiters
}

func (t *PerKeyRoundTripper[K]) RoundTrip(req *http.Request) (*http.Response, error) {
	limiter := t.Limiter(req)
	start := time.Now()
	if err := limiter.Wait(req.Context()); err != nil {
		return nil, err
	}
	wait := time.Since(start)
	defer func() {
		logger := t.Logger
		if logger == nil || logger.Writer() == io.Discard {
			return
		}
		total := time.Since(start)
		logger.Printf(
			"%T - key: %v\twait: %dms\tresp: %dms\ttotal: %dms\treq: %s %s",
			t,
			t.Key(req),
			wait.Milliseconds(),
			(total - wait).Milliseconds(),
			total.Milliseconds(),
			req.Method,
			req.URL.String(),
		)
	}()
	return t.RoundTripper.RoundTrip(req)
}

func PerOriginRoundTripper(
	defaultLimit rate.Limit,
	defaultBurst int,
	roundTripper http.RoundTripper,
) *PerKeyRoundTripper[string] {
	return NewPerKeyRoundTripper(defaultLimit, defaultBurst, TargetOrigin, roundTripper)
}
