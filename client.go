// Package fetch is a minimal, fast HTTP client built for scraping.
// Pure net/http — no framework deps. Stealth via headers, proxy, retry.
//
//	client := fetch.Client{Base: "https://api.example.com"}
//	var user User
//	err := client.Get(ctx, "/users/1").JSON(&user)
package fetch

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

// Client is a zero-config HTTP client. The zero value is usable.
//
//	client := fetch.Client{Base: "https://api.example.com"}
//	client := fetch.Client{Base: "https://api.example.com", Timeout: 10 * time.Second, Retry: 3}
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

	once sync.Once
	hc   *http.Client
}

func (c *Client) init() {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if c.Proxy != "" {
		if proxyURL, err := url.Parse(c.Proxy); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
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

func (c *Client) httpClient() *http.Client {
	c.once.Do(c.init)
	return c.hc
}

// With creates a derived client, overriding non-zero fields.
//
//	sec := client.With(fetch.Override{Base: "https://efts.sec.gov"})
func (c *Client) With(o Override) *Client {
	n := &Client{
		Base:    c.Base,
		Timeout: c.Timeout,
		Retry:   c.Retry,
		Proxy:   c.Proxy,
		Headers: c.Headers,
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
	return n
}

// Override specifies fields to override in Client.With().
type Override struct {
	Base    string
	Timeout time.Duration
	Retry   int
	Proxy   string
	Headers func() Headers
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
			if last != nil {
				var re *ResponseError
				if errors.As(last.err, &re) && re.RetryAfter > 0 {
					wait = re.RetryAfter
				}
			}
			select {
			case <-ctx.Done():
				return &Response{err: ctx.Err(), RequestURL: fullURL}
			case <-time.After(wait):
			}
		}

		last = c.doOnce(ctx, method, fullURL, body)
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

func (c *Client) doOnce(ctx context.Context, method, fullURL string, body any) *Response {
	var bodyReader io.Reader
	var contentType string

	switch b := body.(type) {
	case nil:
	case []byte:
		bodyReader = bytes.NewReader(b)
	case string:
		bodyReader = strings.NewReader(b)
	case io.Reader:
		bodyReader = b
	default:
		data, err := json.Marshal(b)
		if err != nil {
			return &Response{err: fmt.Errorf("fetch: marshal body: %w", err), RequestURL: fullURL}
		}
		bodyReader = bytes.NewReader(data)
		contentType = "application/json"
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return &Response{err: err, RequestURL: fullURL}
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Apply per-request dynamic headers.
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
		}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &Response{
			err:            fmt.Errorf("fetch: read body: %w", err),
			Status:         resp.StatusCode,
			RequestURL:     fullURL,
			RequestHeaders: headersFromHTTP(req.Header),
		}
	}

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
	}

	return r
}

func headersFromHTTP(h http.Header) Headers {
	out := make(Headers, len(h))
	for k, v := range h {
		out[k] = v[0]
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
