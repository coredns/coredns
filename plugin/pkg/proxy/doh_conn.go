package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/miekg/dns"
)

const (
	dnsMessageMimeType    = "application/dns-message"
	maxDNSMessageSize     = 1472
	defaultRequestTimeout = 2 * time.Second
)

var (
	dnsMessageMimeTypeHeader = []string{dnsMessageMimeType}
	errResponseTooLarge      = errors.New("dns response size is too large")
	errInvalidHTTPStatus     = errors.New("invalid http response status code")
	startCleanupOnce         sync.Once

	// Global client cache with mutex for thread safety
	dohClientCache      = make(map[string]cachedDoHClient)
	dohClientCacheMutex sync.RWMutex

	// Global shared HTTP client with appropriate settings for DNS
	sharedHTTPClient = &http.Client{
		Timeout: defaultRequestTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  true, // DNS messages are already compressed
		},
	}
)

type cachedDoHClient struct {
	client   *dohDNSClient
	lastUsed time.Time
	created  time.Time
}

// dohDNSClient is a DNS client that proxies requests to the upstream server using DoH protocol
type dohDNSClient struct {
	client httpRequestDoer
	url    string
}

// httpRequestDoer abstracts the HTTP client's Do method
type httpRequestDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// newDoHDNSClient creates a new instance of dohDNSClient service
// url must be a full URL to send DoH requests to like "https://example.com/dns-query"
func newDoHDNSClient(httpClient httpRequestDoer, url string) *dohDNSClient {
	return &dohDNSClient{httpClient, url}
}

// getOrCreateDoHClient gets an existing client from the cache or creates a new one
func getOrCreateDoHClient(url string) *dohDNSClient {
	//formattedURL := formatDoHURL(url)

	dohClientCacheMutex.RLock()
	_, found := dohClientCache[url]
	dohClientCacheMutex.RUnlock()

	if found {
		dohClientCacheMutex.Lock()
		if entry, stillExists := dohClientCache[url]; stillExists {
			entry.lastUsed = time.Now()
			dohClientCache[url] = entry
			dohClientCacheMutex.Unlock()
			return entry.client
		}
		dohClientCacheMutex.Unlock()
	}

	dohClientCacheMutex.Lock()
	defer dohClientCacheMutex.Unlock()

	// Double-check after acquiring write lock
	if cached, found := dohClientCache[url]; found {
		cached.lastUsed = time.Now()
		dohClientCache[url] = cached
		return cached.client
	}

	client := newDoHDNSClient(sharedHTTPClient, url)
	dohClientCache[url] = cachedDoHClient{
		client:   client,
		lastUsed: time.Now(),
		created:  time.Now(),
	}

	if len(dohClientCache) == 1 {
		startDoHClientCleanup()
	}

	return client
}

// Query sends a DNS query over HTTPS to the configured endpoint
func (c *dohDNSClient) Query(ctx context.Context, dnsreq []byte) (r *dns.Msg, err error) {
	var req *http.Request
	if req, err = http.NewRequestWithContext(ctx, "POST", c.url, bytes.NewReader(dnsreq)); err != nil {
		log.Errorf("(Query) Failed to create HTTP request: %v", err)
		return
	}
	req.Header["Accept"] = dnsMessageMimeTypeHeader
	req.Header["Content-Type"] = dnsMessageMimeTypeHeader
	log.Infof("https request -> %v", req)
	var resp *http.Response
	if resp, err = c.client.Do(req); err != nil {
		log.Errorf("(Query) HTTP request failed: %v", err)
		return
	}
	defer resp.Body.Close()
	log.Infof("https response <- %v", resp)
	// RFC8484 Section 4.2.1:
	// A successful HTTP response with a 2xx status code is used for any valid DNS response,
	// regardless of the DNS response code.
	// HTTP responses with non-successful HTTP status codes do not contain
	// replies to the original DNS question in the HTTP request.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errInvalidHTTPStatus
	}

	// Limit the number of bytes read to avoid potential DoS attacks
	var body []byte
	if body, err = io.ReadAll(io.LimitReader(resp.Body, maxDNSMessageSize+1)); err != nil {
		return
	}
	if len(body) > maxDNSMessageSize {
		return nil, errResponseTooLarge
	}
	r = new(dns.Msg)
	err = r.Unpack(body)
	log.Infof("Unpacked DNS response %v", r)
	if err != nil {
		return
	}
	return
}

// cleanupDoHClientCache removes old entries from the cache
func cleanupDoHClientCache() {
	threshold := time.Now().Add(-30 * time.Minute) // Remove clients not used for 30 minutes

	dohClientCacheMutex.Lock()
	defer dohClientCacheMutex.Unlock()

	for url, entry := range dohClientCache {
		if entry.lastUsed.Before(threshold) {
			delete(dohClientCache, url)
		}
	}
}

func startDoHClientCleanup() {
	startCleanupOnce.Do(func() {
		ticker := time.NewTicker(10 * time.Minute)
		go func() {
			for range ticker.C {
				cleanupDoHClientCache()
			}
		}()
	})
}
