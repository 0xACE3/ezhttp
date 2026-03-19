// Package ezhttp is a minimal, fast HTTP client built for scraping.
// Pure net/http — no framework deps. Stealth via headers, proxy, retry.
//
//	client := ezhttp.Client{Base: "https://api.example.com"}
//	var user User
//	err := client.Get(ctx, "/users/1").JSON(&user)
package ezhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config holds client settings. Pass to New() to create a Client.
// Every field is optional — the zero value gives you a working client.
type Config struct {
	Base    string        // Base URL prepended to relative paths
	Timeout time.Duration // Request timeout (default: 30s)
	Retry   int           // Retry count for 429/5xx (0 = no retries)
	Proxy   string        // Proxy URL (http, https, socks5)
	Browser *Browser      // TLS fingerprint (nil = default Go TLS)
	Headers func() Headers // Dynamic headers applied to every request
	Debug   bool          // Enable structured logging
}

// New creates a Client from Config.
//
//	c := ezhttp.New(ezhttp.Config{Base: "https://api.example.com", Retry: 2})
//	var user User
//	err := c.Get(ctx, "/users/1").JSON(&user)
func New(cfg Config) *Client {
	return &Client{
		Base:    cfg.Base,
		Timeout: cfg.Timeout,
		Retry:   cfg.Retry,
		Proxy:   cfg.Proxy,
		Headers: cfg.Headers,
		Browser: cfg.Browser,
		Debug:   cfg.Debug,
	}
}

// NewWithHeaders is a shortcut: New() + static headers map.
//
//	c := ezhttp.NewWithHeaders("https://api.example.com", map[string]string{"Authorization": "Bearer xxx"})
func NewWithHeaders(base string, headers map[string]string) *Client {
	return New(Config{
		Base: base,
		Headers: func() Headers { return Headers(headers) },
	})
}

// NewClient creates a Client from this config with a custom base and dynamic headers.
func (c Config) NewClient(base string, headers func() Headers) *Client {
	c.Base = base
	c.Headers = headers
	return New(c)
}

// NewWithHeaders creates a Client from this config with a custom base and static headers.
func (c Config) NewWithHeaders(base string, headers map[string]string) *Client {
	return c.NewClient(base, func() Headers { return Headers(headers) })
}

// Client is a zero-config HTTP client. The zero value is usable.
//
//	client := ezhttp.Client{Base: "https://api.example.com"}
//	client := ezhttp.Client{Base: "https://api.example.com", Timeout: 10 * time.Second, Retry: 3}
type Client struct {
	// Base URL prepended to relative paths. Ignored if path starts with http.
	Base string

	// Timeout for the entire request. Default: 30s.
	Timeout time.Duration

	// Retry count for 429/5xx/network errors. 0 = no retries.
	Retry int

	// Proxy URL (http, https, socks5).
	Proxy string

	// Headers returns headers applied to every request. Called per-request,
	// so it can return dynamic values (timestamps, rotating tokens).
	Headers func() Headers

	// Browser enables TLS fingerprinting and browser header impersonation.
	// Use pre-built profiles: Chrome, Firefox, Safari, Edge, or RandomBrowser().
	// nil = no fingerprinting (default Go TLS).
	Browser *Browser

	// ForceHTTP selects the HTTP protocol version.
	// Auto (default): HTTP/2 with fingerprint, standard Go negotiation without.
	// HTTP1, HTTP2, HTTP3 force a specific version.
	ForceHTTP Version

	// Debug enables structured logging to stderr.
	// Logs method, status, latency, size, URL, protocol, browser per request.
	Debug bool

	once sync.Once
	hc   *http.Client
}

func (c *Client) init() {
	transport := c.buildTransport()
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	jar, _ := cookiejar.New(nil)
	c.hc = &http.Client{
		Transport: transport,
		Timeout:   timeout,
		Jar:       jar,
	}
}

func (c *Client) buildTransport() http.RoundTripper {
	if c.Browser != nil {
		return c.fingerprintedTransport()
	}
	return c.standardTransportForVersion()
}

func (c *Client) fingerprintedTransport() http.RoundTripper {
	switch c.ForceHTTP {
	case HTTP1:
		return fingerprintH1Transport(c.Browser, c.Proxy)
	case HTTP2:
		return fingerprintH2Transport(c.Browser, c.Proxy)
	case HTTP3:
		return h3RoundTripper()
	default: // Auto — h2 for HTTPS (like real browsers), h1 for plain HTTP
		return &multiTransport{
			https: fingerprintH2Transport(c.Browser, c.Proxy),
			http:  fingerprintH1Transport(c.Browser, c.Proxy),
		}
	}
}

func (c *Client) standardTransportForVersion() http.RoundTripper {
	switch c.ForceHTTP {
	case HTTP1:
		return standardH1Transport(c.Proxy)
	case HTTP2:
		t := standardTransport(c.Proxy)
		t.ForceAttemptHTTP2 = true
		return t
	case HTTP3:
		return h3RoundTripper()
	default: // Auto — Go's default h1/h2 negotiation
		return standardTransport(c.Proxy)
	}
}

func standardTransport(proxyStr string) *http.Transport {
	t := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if proxyStr != "" {
		if proxyURL, err := url.Parse(proxyStr); err == nil {
			t.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return t
}

func (c *Client) httpClient() *http.Client {
	c.once.Do(c.init)
	return c.hc
}

// With creates a derived client, overriding non-zero fields.
//
//	sec := client.With(fetch.Override{Base: "https://efts.sec.gov"})
func (c *Client) With(o Override) *Client {
	n := &Client{
		Base:      c.Base,
		Timeout:   c.Timeout,
		Retry:     c.Retry,
		Proxy:     c.Proxy,
		Headers:   c.Headers,
		Browser:   c.Browser,
		ForceHTTP: c.ForceHTTP,
		Debug:     c.Debug,
	}
	if o.Base != "" {
		n.Base = o.Base
	}
	if o.Timeout != 0 {
		n.Timeout = o.Timeout
	}
	if o.Retry != 0 {
		n.Retry = o.Retry
	}
	if o.Proxy != "" {
		n.Proxy = o.Proxy
	}
	if o.Headers != nil {
		n.Headers = o.Headers
	}
	if o.Browser != nil {
		n.Browser = o.Browser
	}
	if o.ForceHTTP != 0 {
		n.ForceHTTP = o.ForceHTTP
	}
	if o.Debug {
		n.Debug = o.Debug
	}
	return n
}

// Override specifies fields to override in Client.With().
type Override struct {
	Base      string
	Timeout   time.Duration
	Retry     int
	Proxy     string
	Headers   func() Headers
	Browser   *Browser
	ForceHTTP Version
	Debug     bool
}

// --- HTTP methods ---

func (c *Client) Get(ctx context.Context, path string) *Response {
	return c.exec(ctx, http.MethodGet, path, nil)
}

func (c *Client) Post(ctx context.Context, path string, body any) *Response {
	return c.exec(ctx, http.MethodPost, path, body)
}

func (c *Client) Put(ctx context.Context, path string, body any) *Response {
	return c.exec(ctx, http.MethodPut, path, body)
}

func (c *Client) Patch(ctx context.Context, path string, body any) *Response {
	return c.exec(ctx, http.MethodPatch, path, body)
}

func (c *Client) Delete(ctx context.Context, path string) *Response {
	return c.exec(ctx, http.MethodDelete, path, nil)
}

func (c *Client) Head(ctx context.Context, path string) *Response {
	return c.exec(ctx, http.MethodHead, path, nil)
}

// --- internal ---

func (c *Client) exec(ctx context.Context, method, path string, body any) *Response {
	fullURL := resolveURL(c.Base, path)

	maxAttempts := c.Retry + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var last *Response
	for attempt := range maxAttempts {
		if attempt > 0 {
			wait := backoff(attempt)
			lastStatus := 0
			if last != nil {
				var re *ResponseError
				if errors.As(last.err, &re) {
					lastStatus = re.Status
					if re.RetryAfter > 0 {
						wait = re.RetryAfter
					}
				}
			}
			c.logRetry(method, fullURL, attempt, maxAttempts-1, wait, lastStatus)
			select {
			case <-ctx.Done():
				return &Response{err: ctx.Err(), RequestURL: fullURL}
			case <-time.After(wait):
			}
		}

		start := time.Now()
		var reqSize int
		last, reqSize = c.doOnce(ctx, method, fullURL, body)
		c.logRequest(method, fullURL, last.Status, time.Since(start), reqSize, len(last.Body), last.err)

		if last.err == nil {
			return last
		}

		// Only retry on network errors, 429, or 5xx.
		var re *ResponseError
		if errors.As(last.err, &re) {
			if re.Status != 429 && re.Status < 500 {
				return last // 4xx (non-429), don't retry
			}
		}
	}
	return last
}

func (c *Client) doOnce(ctx context.Context, method, fullURL string, body any) (*Response, int) {
	var bodyReader io.Reader
	var contentType string
	var reqSize int

	switch b := body.(type) {
	case nil:
	case []byte:
		reqSize = len(b)
		bodyReader = bytes.NewReader(b)
	case string:
		reqSize = len(b)
		bodyReader = strings.NewReader(b)
	case io.Reader:
		bodyReader = b
	default:
		data, err := json.Marshal(b)
		if err != nil {
			return &Response{err: fmt.Errorf("ezhttp: marshal body: %w", err), RequestURL: fullURL}, 0
		}
		reqSize = len(data)
		bodyReader = bytes.NewReader(data)
		contentType = "application/json"
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return &Response{err: err, RequestURL: fullURL}, 0
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Apply browser headers first (UA, sec-ch-ua, accept, etc.)
	if c.Browser != nil {
		for k, v := range c.Browser.browserHeaders() {
			req.Header.Set(k, v)
		}
	}

	// Apply per-request dynamic headers (override browser headers).
	if c.Headers != nil {
		for k, v := range c.Headers() {
			req.Header.Set(k, v)
		}
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return &Response{
			err:            err,
			RequestURL:     fullURL,
			RequestHeaders: headersFromHTTP(req.Header),
		}, reqSize
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &Response{
			err:            fmt.Errorf("ezhttp: read body: %w", err),
			Status:         resp.StatusCode,
			RequestURL:     fullURL,
			RequestHeaders: headersFromHTTP(req.Header),
		}, reqSize
	}

	// Auto-decompress when we set Accept-Encoding manually (browser fingerprint).
	// net/http only auto-decompresses when it sets the header itself.
	respBody, decompErr := decompressBody(respBody, resp.Header.Get("Content-Encoding"))

	r := &Response{
		Status:         resp.StatusCode,
		Body:           respBody,
		Headers:        headersFromHTTP(resp.Header),
		RequestHeaders: headersFromHTTP(req.Header),
		RequestURL:     fullURL,
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		r.err = &ResponseError{
			Status:     resp.StatusCode,
			Body:       respBody,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	} else if decompErr != nil {
		r.err = decompErr
	}

	return r, reqSize
}

func headersFromHTTP(h http.Header) Headers {
	out := make(Headers, len(h))
	for k, v := range h {
		out[k] = strings.Join(v, ", ")
	}
	return out
}

func parseRetryAfter(val string) time.Duration {
	if val == "" {
		return 0
	}
	if secs, err := strconv.Atoi(val); err == nil {
		return time.Duration(secs) * time.Second
	}
	if t, err := time.Parse(time.RFC1123, val); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}

func backoff(attempt int) time.Duration {
	base := time.Duration(1<<uint(attempt)) * 500 * time.Millisecond
	if base > 30*time.Second {
		base = 30 * time.Second
	}
	jitter := time.Duration(rand.Int64N(int64(base / 2)))
	return base + jitter
}

func decompressBody(body []byte, encoding string) ([]byte, error) {
	if len(body) == 0 || encoding == "" {
		return body, nil
	}
	switch strings.ToLower(encoding) {
	case "gzip":
		decoded, err := DecodeGzip(body)
		if err != nil {
			return body, fmt.Errorf("ezhttp: decompress gzip: %w", err)
		}
		return decoded, nil
	case "br":
		decoded, err := DecodeBrotli(body)
		if err != nil {
			return body, fmt.Errorf("ezhttp: decompress brotli: %w", err)
		}
		return decoded, nil
	}
	return body, nil
}
