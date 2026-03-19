package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	fetch "github.com/0xACE3/ezhttp"
)

// These tests verify that our browser fingerprinting passes server-side
// and CDN-level bot detection. The actual bot detection on these sites
// is client-side JS (navigator.webdriver, headless Chrome signals, etc.)
// which doesn't apply to HTTP clients — but we must not be blocked
// before the page is served.

// ===========================
// 1. BROWSERSCAN.NET
// ===========================

func TestBotDetect_BrowserScan(t *testing.T) {
	client := fetch.Client{
		Timeout: 15 * time.Second,
		Browser: fetch.Chrome,
	}

	doc, err := client.Get(context.Background(), "https://www.browserscan.net/bot-detection").HTML()
	if err != nil {
		t.Skipf("browserscan.net unavailable: %v", err)
	}

	// Should get the full SPA page, not a bot challenge.
	title := doc.Find("title").Text()
	if title == "" {
		t.Fatal("expected page title, got empty (likely blocked)")
	}
	t.Logf("BrowserScan title: %s", title)

	// The page should contain the app root (React/Vue SPA).
	html := doc.Find("body").Text()
	if strings.Contains(strings.ToLower(html), "access denied") ||
		strings.Contains(strings.ToLower(html), "blocked") {
		t.Fatal("bot detection blocked us")
	}
}

// ===========================
// 2. PIXELSCAN.NET (CLOUDFLARE)
// ===========================

func TestBotDetect_PixelScan(t *testing.T) {
	client := fetch.Client{
		Timeout: 15 * time.Second,
		Browser: fetch.Chrome,
	}

	resp := client.Get(context.Background(), "https://pixelscan.net/bot-check")
	if resp.Err() != nil {
		t.Skipf("pixelscan.net unavailable: %v", resp.Err())
	}

	// Must not get Cloudflare challenge page.
	body := resp.Text()
	if strings.Contains(body, "cf-challenge") ||
		strings.Contains(body, "Checking your browser") {
		t.Fatal("Cloudflare challenge detected — fingerprint not passing")
	}

	doc, err := resp.HTML()
	if err != nil {
		t.Fatal(err)
	}

	title := doc.Find("title").Text()
	if title == "" {
		t.Fatal("expected page title")
	}
	t.Logf("PixelScan title: %s", title)
	t.Logf("PixelScan body size: %d bytes", len(body))
}

// ===========================
// 3. DEVICEANDBROWSERINFO.COM
// ===========================

func TestBotDetect_DeviceAndBrowserInfo(t *testing.T) {
	// This server has h2 framing issues, use HTTP/1.1.
	client := fetch.Client{
		Timeout:   15 * time.Second,
		Browser:   fetch.Chrome,
		ForceHTTP: fetch.HTTP1,
	}

	doc, err := client.Get(context.Background(), "https://deviceandbrowserinfo.com/are_you_a_bot").HTML()
	if err != nil {
		t.Skipf("deviceandbrowserinfo.com unavailable: %v", err)
	}

	title := doc.Find("title").Text()
	if title == "" {
		t.Fatal("expected page title, got empty (likely blocked)")
	}

	body := doc.Find("body").Text()
	if strings.Contains(strings.ToLower(body), "access denied") {
		t.Fatal("bot detection blocked us")
	}

	t.Logf("DeviceAndBrowserInfo title: %s", title)
}

// ===========================
// 4. ALL BROWSERS PASS CLOUDFLARE
// ===========================

func TestBotDetect_AllBrowsers_Cloudflare(t *testing.T) {
	browsers := map[string]*fetch.Browser{
		"Chrome":  fetch.Chrome,
		"Firefox": fetch.Firefox,
		"Safari":  fetch.Safari,
		"Edge":    fetch.Edge,
	}

	for name, browser := range browsers {
		t.Run(name, func(t *testing.T) {
			client := fetch.Client{
				Timeout: 15 * time.Second,
				Browser: browser,
			}

			resp := client.Get(context.Background(), "https://pixelscan.net/bot-check")
			if resp.Err() != nil {
				t.Skipf("pixelscan.net unavailable: %v", resp.Err())
			}

			body := resp.Text()
			if strings.Contains(body, "cf-challenge") ||
				strings.Contains(body, "Checking your browser") {
				t.Fatalf("%s fingerprint triggered Cloudflare challenge", name)
			}
			t.Logf("%s passed Cloudflare — %d bytes", name, len(body))
		})
	}
}

// ===========================
// 5. COMPARE: NO FINGERPRINT GETS BLOCKED
// ===========================

func TestBotDetect_NoFingerprint_Comparison(t *testing.T) {
	// Without fingerprinting, some CDNs may treat us differently.
	client := fetch.Client{
		Timeout: 15 * time.Second,
		// No Browser — Go default TLS + UA.
	}

	resp := client.Get(context.Background(), "https://www.browserscan.net/bot-detection")
	if resp.Err() != nil {
		t.Skipf("browserscan.net unavailable: %v", resp.Err())
	}

	// Log the difference — Go default UA is obvious.
	ua := "Go-http-client"
	t.Logf("No fingerprint — status=%d, body=%d bytes (UA: %s)", resp.Status, len(resp.Body), ua)
}
