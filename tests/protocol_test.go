package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	fetch "github.com/0xACE3/ezhttp"
)

// ===========================
// 1. HTTP/1.1 EXPLICIT
// ===========================

func TestProtocol_HTTP1(t *testing.T) {
	client := fetch.Client{
		Timeout:   15 * time.Second,
		Browser:   fetch.Chrome,
		ForceHTTP: fetch.HTTP1,
	}

	resp := client.Get(context.Background(), "https://httpbin.org/headers")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	ua := resp.Path("headers", "User-Agent").String()
	if !strings.Contains(ua, "Chrome") {
		t.Fatalf("expected Chrome UA, got %q", ua)
	}
	t.Logf("HTTP/1.1 forced — UA: %s", ua)
}

// ===========================
// 2. HTTP/2 EXPLICIT
// ===========================

func TestProtocol_HTTP2(t *testing.T) {
	client := fetch.Client{
		Timeout:   15 * time.Second,
		Browser:   fetch.Chrome,
		ForceHTTP: fetch.HTTP2,
	}

	resp := client.Get(context.Background(), "https://httpbin.org/headers")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	ua := resp.Path("headers", "User-Agent").String()
	if !strings.Contains(ua, "Chrome") {
		t.Fatalf("expected Chrome UA, got %q", ua)
	}
	t.Logf("HTTP/2 forced — UA: %s", ua)
}

// ===========================
// 3. HTTP/2 WITHOUT FINGERPRINT
// ===========================

func TestProtocol_HTTP2_NoFingerprint(t *testing.T) {
	client := fetch.Client{
		Timeout:   15 * time.Second,
		ForceHTTP: fetch.HTTP2,
	}

	resp := client.Get(context.Background(), "https://httpbin.org/headers")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	ua := resp.Path("headers", "User-Agent").String()
	t.Logf("HTTP/2 no fingerprint — UA: %s", ua)
}

// ===========================
// 4. AUTO (DEFAULT) WITH FINGERPRINT = H2
// ===========================

func TestProtocol_Auto_Fingerprint(t *testing.T) {
	client := fetch.Client{
		Timeout: 15 * time.Second,
		Browser: fetch.Chrome,
		// ForceHTTP: fetch.Auto (default)
	}

	resp := client.Get(context.Background(), "https://httpbin.org/headers")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	ua := resp.Path("headers", "User-Agent").String()
	if !strings.Contains(ua, "Chrome") {
		t.Fatalf("expected Chrome UA, got %q", ua)
	}

	secChua := resp.Path("headers", "Sec-Ch-Ua").String()
	if secChua == "" {
		t.Fatal("expected Sec-Ch-Ua header")
	}
	t.Logf("Auto+fingerprint (h2) — UA: %s", ua)
}

// ===========================
// 5. HTTP/2 FINGERPRINT - GITHUB SCRAPE
// ===========================

func TestProtocol_HTTP2_GitHub(t *testing.T) {
	client := fetch.Client{
		Timeout:   15 * time.Second,
		Browser:   fetch.Chrome,
		ForceHTTP: fetch.HTTP2,
	}

	doc, err := client.Get(context.Background(), "https://github.com/golang/go").HTML()
	if err != nil {
		t.Fatal(err)
	}

	title := doc.Find("title").Text()
	if !strings.Contains(strings.ToLower(title), "go") {
		t.Fatalf("unexpected title: %q", title)
	}
	t.Logf("HTTP/2 GitHub scrape — title: %s", title)
}

// ===========================
// 6. HTTP/2 FINGERPRINT - JSON API
// ===========================

func TestProtocol_HTTP2_JSONApi(t *testing.T) {
	client := fetch.Client{
		Timeout:   15 * time.Second,
		Browser:   fetch.Chrome,
		ForceHTTP: fetch.HTTP2,
		Headers: func() fetch.Headers {
			return fetch.Headers{
				"Accept": "application/json",
			}
		},
	}

	resp := client.Get(context.Background(), "https://api.github.com/repos/golang/go")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	name := resp.Path("full_name").String()
	if name != "golang/go" {
		t.Fatalf("expected golang/go, got %q", name)
	}
	t.Logf("HTTP/2 JSON API — %s (%d stars)", name, resp.Path("stargazers_count").Int())
}

// ===========================
// 7. HTTP/3 BASIC
// ===========================

func TestProtocol_HTTP3(t *testing.T) {
	client := fetch.Client{
		Timeout:   15 * time.Second,
		ForceHTTP: fetch.HTTP3,
	}

	// Google supports HTTP/3.
	resp := client.Get(context.Background(), "https://www.google.com")
	if resp.Err() != nil {
		t.Skipf("HTTP/3 unavailable: %v", resp.Err())
	}

	if resp.Status != 200 {
		t.Fatalf("expected 200, got %d", resp.Status)
	}
	t.Logf("HTTP/3 — status=%d, body=%d bytes", resp.Status, len(resp.Body))
}

// ===========================
// 8. HTTP/3 WITH BROWSER HEADERS
// ===========================

func TestProtocol_HTTP3_BrowserHeaders(t *testing.T) {
	client := fetch.Client{
		Timeout:   15 * time.Second,
		Browser:   fetch.Chrome,
		ForceHTTP: fetch.HTTP3,
	}

	// cloudflare-quic.com supports HTTP/3.
	resp := client.Get(context.Background(), "https://cloudflare-quic.com")
	if resp.Err() != nil {
		t.Skipf("HTTP/3 unavailable: %v", resp.Err())
	}

	if resp.Status != 200 {
		t.Fatalf("expected 200, got %d", resp.Status)
	}
	t.Logf("HTTP/3 with browser headers — status=%d", resp.Status)
}

// ===========================
// 9. HTTP/1.1 FORCED - NO FINGERPRINT
// ===========================

func TestProtocol_HTTP1_NoFingerprint(t *testing.T) {
	client := fetch.Client{
		Timeout:   15 * time.Second,
		ForceHTTP: fetch.HTTP1,
	}

	resp := client.Get(context.Background(), "https://httpbin.org/headers")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	ua := resp.Path("headers", "User-Agent").String()
	// Should be Go default UA (no browser fingerprint).
	t.Logf("HTTP/1.1 no fingerprint — UA: %s", ua)
}

// ===========================
// 10. PROTOCOL SWITCH VIA WITH()
// ===========================

func TestProtocol_WithOverride(t *testing.T) {
	base := fetch.Client{
		Timeout: 15 * time.Second,
		Browser: fetch.Chrome,
		// Default: Auto (h2)
	}

	// Override to force HTTP/1.1.
	h1 := base.With(fetch.Override{ForceHTTP: fetch.HTTP1})

	resp := h1.Get(context.Background(), "https://httpbin.org/headers")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	ua := resp.Path("headers", "User-Agent").String()
	if !strings.Contains(ua, "Chrome") {
		t.Fatalf("expected Chrome UA after With(), got %q", ua)
	}
	t.Logf("With() override to HTTP/1.1 — UA: %s", ua)
}

// ===========================
// 11. ALL BROWSERS WORK ON HTTP/2
// ===========================

func TestProtocol_HTTP2_AllBrowsers(t *testing.T) {
	browsers := map[string]*fetch.Browser{
		"Chrome":  fetch.Chrome,
		"Firefox": fetch.Firefox,
		"Safari":  fetch.Safari,
		"Edge":    fetch.Edge,
	}

	for name, browser := range browsers {
		t.Run(name, func(t *testing.T) {
			client := fetch.Client{
				Timeout:   15 * time.Second,
				Browser:   browser,
				ForceHTTP: fetch.HTTP2,
			}

			resp := client.Get(context.Background(), "https://httpbin.org/headers")
			if resp.Err() != nil {
				t.Fatal(resp.Err())
			}

			ua := resp.Path("headers", "User-Agent").String()
			if ua == "" || ua == "Go-http-client/2.0" {
				t.Fatalf("expected browser UA, got %q", ua)
			}
			t.Logf("%s via HTTP/2 — UA: %s", name, ua)
		})
	}
}
