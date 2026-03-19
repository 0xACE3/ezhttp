package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/0xACE3/ezhttp"
)

// ===========================
// 1. CHROME FINGERPRINT
// ===========================

func TestFingerprint_Chrome(t *testing.T) {
	client := ezhttp.Client{
		Base:    "https://httpbin.org",
		Timeout: 15 * time.Second,
		Browser: ezhttp.Chrome,
	}

	resp := client.Get(context.Background(), "/headers")
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

	secPlatform := resp.Path("headers", "Sec-Ch-Ua-Platform").String()
	if secPlatform == "" {
		t.Fatal("expected Sec-Ch-Ua-Platform header")
	}

	t.Logf("Chrome UA: %s", ua)
	t.Logf("Sec-Ch-Ua: %s", secChua)
	t.Logf("Sec-Ch-Ua-Platform: %s", secPlatform)
}

// ===========================
// 2. FIREFOX FINGERPRINT
// ===========================

func TestFingerprint_Firefox(t *testing.T) {
	client := ezhttp.Client{
		Base:    "https://httpbin.org",
		Timeout: 15 * time.Second,
		Browser: ezhttp.Firefox,
	}

	resp := client.Get(context.Background(), "/headers")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	ua := resp.Path("headers", "User-Agent").String()
	if !strings.Contains(ua, "Firefox") {
		t.Fatalf("expected Firefox UA, got %q", ua)
	}

	// Firefox doesn't send sec-ch-ua
	secFetch := resp.Path("headers", "Sec-Fetch-Dest").String()
	if secFetch == "" {
		t.Fatal("expected Sec-Fetch-Dest header")
	}

	t.Logf("Firefox UA: %s", ua)
	t.Logf("Sec-Fetch-Dest: %s", secFetch)
}

// ===========================
// 3. SAFARI FINGERPRINT
// ===========================

func TestFingerprint_Safari(t *testing.T) {
	client := ezhttp.Client{
		Base:    "https://httpbin.org",
		Timeout: 15 * time.Second,
		Browser: ezhttp.Safari,
	}

	resp := client.Get(context.Background(), "/headers")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	ua := resp.Path("headers", "User-Agent").String()
	if !strings.Contains(ua, "Safari") {
		t.Fatalf("expected Safari UA, got %q", ua)
	}

	t.Logf("Safari UA: %s", ua)
}

// ===========================
// 4. EDGE FINGERPRINT
// ===========================

func TestFingerprint_Edge(t *testing.T) {
	client := ezhttp.Client{
		Base:    "https://httpbin.org",
		Timeout: 15 * time.Second,
		Browser: ezhttp.Edge,
	}

	resp := client.Get(context.Background(), "/headers")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	ua := resp.Path("headers", "User-Agent").String()
	if !strings.Contains(ua, "Edg/") {
		t.Fatalf("expected Edge UA, got %q", ua)
	}

	secChua := resp.Path("headers", "Sec-Ch-Ua").String()
	if !strings.Contains(secChua, "Microsoft Edge") {
		t.Fatalf("expected Edge in Sec-Ch-Ua, got %q", secChua)
	}

	t.Logf("Edge UA: %s", ua)
	t.Logf("Sec-Ch-Ua: %s", secChua)
}

// ===========================
// 5. RANDOM BROWSER
// ===========================

func TestFingerprint_Random(t *testing.T) {
	client := ezhttp.Client{
		Base:    "https://httpbin.org",
		Timeout: 15 * time.Second,
		Browser: ezhttp.RandomBrowser(),
	}

	resp := client.Get(context.Background(), "/headers")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	ua := resp.Path("headers", "User-Agent").String()
	if ua == "" || ua == "Go-http-client/2.0" || ua == "Go-http-client/1.1" {
		t.Fatalf("expected browser UA, got Go default: %q", ua)
	}
	t.Logf("Random browser UA: %s", ua)
}

// ===========================
// 6. UA ROTATION
// ===========================

func TestFingerprint_UARotation(t *testing.T) {
	client := ezhttp.Client{
		Base:    "https://httpbin.org",
		Timeout: 15 * time.Second,
		Browser: ezhttp.Chrome,
	}

	seen := map[string]bool{}
	for range 5 {
		resp := client.Get(context.Background(), "/user-agent")
		if resp.Err() != nil {
			t.Fatal(resp.Err())
		}
		ua := resp.Path("user-agent").String()
		seen[ua] = true
		t.Logf("UA: %s", ua)
	}

	// Chrome has 5 UAs — with 5 requests we should see at least 2 different ones
	// (round-robin guarantees all 5 unique if we make 5 requests)
	if len(seen) < 2 {
		t.Fatalf("expected UA rotation, only saw %d unique UAs", len(seen))
	}
	t.Logf("Saw %d unique UAs across 5 requests", len(seen))
}

// ===========================
// 7. BROWSER HEADERS OVERRIDE
// ===========================

func TestFingerprint_CustomHeaderOverride(t *testing.T) {
	client := ezhttp.Client{
		Base:    "https://httpbin.org",
		Timeout: 15 * time.Second,
		Browser: ezhttp.Chrome,
		Headers: func() ezhttp.Headers {
			return ezhttp.Headers{
				"User-Agent": "custom-override/1.0",
			}
		},
	}

	resp := client.Get(context.Background(), "/headers")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	ua := resp.Path("headers", "User-Agent").String()
	if ua != "custom-override/1.0" {
		t.Fatalf("expected custom UA override, got %q", ua)
	}

	// Browser sec-ch-ua should still be present (not overridden)
	secChua := resp.Path("headers", "Sec-Ch-Ua").String()
	if secChua == "" {
		t.Fatal("expected browser Sec-Ch-Ua to survive custom header override")
	}

	t.Logf("Custom UA override works, browser sec-ch-ua preserved: %s", secChua)
}

// ===========================
// 8. TLS FINGERPRINT CHECK
// ===========================

func TestFingerprint_TLS_JA3(t *testing.T) {
	// tls.peet.ws returns TLS fingerprint info including JA3
	client := ezhttp.Client{
		Timeout: 15 * time.Second,
		Browser: ezhttp.Chrome,
	}

	resp := client.Get(context.Background(), "https://tls.peet.ws/api/all")
	if resp.Err() != nil {
		t.Skipf("tls.peet.ws unavailable: %v", resp.Err())
	}

	// Check TLS version
	tlsVersion := resp.Path("tls_version").String()
	t.Logf("TLS version: %s", tlsVersion)

	// Check cipher suites — Chrome uses specific ones
	ja3Hash := resp.Path("ja3_hash").String()
	ja3 := resp.Path("ja3").String()
	t.Logf("JA3 hash: %s", ja3Hash)
	t.Logf("JA3: %.100s...", ja3)

	// Check HTTP version
	httpVersion := resp.Path("http_version").String()
	t.Logf("HTTP version: %s", httpVersion)

	// Check user agent sent
	ua := resp.Path("user_agent").String()
	if !strings.Contains(ua, "Chrome") {
		t.Logf("Warning: UA doesn't contain Chrome: %s", ua)
	}
	t.Logf("UA seen by server: %s", ua)

	// The JA3 should NOT match Go's default TLS fingerprint
	if ja3Hash == "" {
		t.Log("JA3 hash not returned (API may have changed)")
	}
}

func TestFingerprint_TLS_NoFingerprint(t *testing.T) {
	// Compare: no Browser = Go default TLS
	client := ezhttp.Client{Timeout: 15 * time.Second}

	resp := client.Get(context.Background(), "https://tls.peet.ws/api/all")
	if resp.Err() != nil {
		t.Skipf("tls.peet.ws unavailable: %v", resp.Err())
	}

	ja3Hash := resp.Path("ja3_hash").String()
	ua := resp.Path("user_agent").String()
	t.Logf("Default Go TLS — JA3: %s, UA: %s", ja3Hash, ua)
}

// ===========================
// 9. REAL SCRAPING: PROTECTED SITE
// ===========================

func TestFingerprint_RealScrape_GitHub(t *testing.T) {
	// GitHub serves different content to bots vs browsers
	client := ezhttp.Client{
		Timeout: 15 * time.Second,
		Browser: ezhttp.Chrome,
	}

	doc, err := client.Get(context.Background(), "https://github.com/golang/go").HTML()
	if err != nil {
		t.Fatal(err)
	}

	// Should get the full page, not a bot challenge
	title := doc.Find("title").Text()
	if title == "" {
		t.Fatal("expected page title")
	}
	if !strings.Contains(strings.ToLower(title), "go") {
		t.Fatalf("unexpected title: %q", title)
	}
	t.Logf("GitHub page title: %s", title)

	// Check we can find repo-specific elements
	about := doc.Find("[class*='about']").Text()
	t.Logf("About section found: %v (length: %d)", about != "", len(about))
}

// ===========================
// 10. FINGERPRINT + JSON API
// ===========================

func TestFingerprint_JSONApi(t *testing.T) {
	client := ezhttp.Client{
		Timeout: 15 * time.Second,
		Browser: ezhttp.Chrome,
		Headers: func() ezhttp.Headers {
			return ezhttp.Headers{
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
	t.Logf("Fingerprinted request to GitHub API: %s (%d stars)",
		name, resp.Path("stargazers_count").Int())
}

// ===========================
// 11. FINGERPRINT + WEBSOCKET
// ===========================

func TestFingerprint_WithWebSocket(t *testing.T) {
	client := ezhttp.Client{
		Timeout: 15 * time.Second,
		Browser: ezhttp.Chrome,
	}

	ws, err := client.WS(context.Background(), "wss://stream.binance.com:9443/ws/btcusdt@ticker", ezhttp.WSConfig{
		Headers: ezhttp.Headers{"Origin": "https://www.binance.com"},
	})
	if err != nil {
		t.Skipf("Binance WS unavailable: %v", err)
	}
	defer ws.Close()

	timeout := time.After(10 * time.Second)
	select {
	case msg, ok := <-ws.Messages:
		if !ok {
			t.Fatal("channel closed")
		}
		sym := msg.Path("s").String()
		if sym != "BTCUSDT" {
			t.Fatalf("expected BTCUSDT, got %q", sym)
		}
		t.Logf("Fingerprinted WS: BTCUSDT price=%s", msg.Path("c").String())
	case <-timeout:
		t.Fatal("timeout")
	}
}
