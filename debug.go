package ezhttp

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// debugLog prints a single-line structured log to stderr when Debug is true.
//
// Format:
//
//	[api.example.com] GET  200  142ms  ← 3.2kB  /users/1  h2 chrome
//	[api.example.com] POST 201   89ms  → 52B ← 124B  /users
//	[api.example.com] GET  429  302ms  ← 98B   /data  | retry 1/3 after=30s
//	[down.example.com] GET  ---  ERR            /health  | err: connection refused
//	[stream.binance.com] WS  open              /ws/btcusdt@ticker  chrome
func (c *Client) logRequest(method, fullURL string, status int, latency time.Duration, reqSize, respSize int, err error) {
	if !c.Debug {
		return
	}

	host, path := splitURL(fullURL)

	var b strings.Builder
	fmt.Fprintf(&b, "[%s] ", host)

	// Method
	fmt.Fprintf(&b, "%-5s", method)

	// Status or ERR
	if err != nil && status == 0 {
		b.WriteString("---  ")
	} else {
		fmt.Fprintf(&b, "%-5d", status)
	}

	// Latency
	fmt.Fprintf(&b, "%-8s", formatDuration(latency))

	// Size: → sent (only for POST/PUT/PATCH), ← received
	if reqSize > 0 {
		fmt.Fprintf(&b, "→ %-8s", formatBytes(reqSize))
	}
	fmt.Fprintf(&b, "← %-9s", formatBytes(respSize))

	// Path (not full URL)
	b.WriteString(truncPath(path, 60))

	// Tags: protocol + browser
	tags := c.debugTags()
	if tags != "" {
		b.WriteString("  ")
		b.WriteString(tags)
	}

	// Error detail
	if err != nil {
		b.WriteString("  | err: ")
		b.WriteString(compactErr(err))
	}

	fmt.Fprintln(os.Stderr, b.String())
}

func (c *Client) logRetry(method, fullURL string, attempt, max int, wait time.Duration, status int) {
	if !c.Debug {
		return
	}
	host, path := splitURL(fullURL)
	var reason string
	switch {
	case status == 429:
		reason = "rate-limited"
	case status >= 500:
		reason = fmt.Sprintf("server-error(%d)", status)
	default:
		reason = "network-error"
	}
	fmt.Fprintf(os.Stderr, "[%s] %-5s%-5s%-8s        %s  | retry %d/%d %s\n",
		host, method, "...", formatDuration(wait), truncPath(path, 60), attempt, max, reason)
}

func (c *Client) logWS(event, fullURL string) {
	if !c.Debug {
		return
	}
	host, path := splitURL(fullURL)
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] WS   ", host)
	fmt.Fprintf(&b, "%-21s", event)
	b.WriteString(truncPath(path, 60))

	tags := c.debugTags()
	if tags != "" {
		b.WriteString("  ")
		b.WriteString(tags)
	}
	fmt.Fprintln(os.Stderr, b.String())
}

func (c *Client) debugTags() string {
	var parts []string
	switch c.ForceHTTP {
	case HTTP1:
		parts = append(parts, "h1")
	case HTTP2:
		parts = append(parts, "h2")
	case HTTP3:
		parts = append(parts, "h3")
	default:
		if c.Browser != nil {
			parts = append(parts, "h2") // auto with fingerprint = h2
		}
	}
	if c.Browser != nil {
		parts = append(parts, c.Browser.name)
	}
	return strings.Join(parts, " ")
}

// --- formatters ---

func splitURL(fullURL string) (host, path string) {
	u, err := url.Parse(fullURL)
	if err != nil {
		return "???", fullURL
	}
	host = u.Host
	path = u.RequestURI()
	if path == "" {
		path = "/"
	}
	return
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dus", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
}

func formatBytes(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fkB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	}
}

func truncPath(path string, max int) string {
	if len(path) <= max {
		return path
	}
	return path[:max-3] + "..."
}

func compactErr(err error) string {
	s := err.Error()
	for _, prefix := range []string{
		"Get ", "Post ", "Put ", "Patch ", "Delete ", "Head ",
	} {
		if strings.HasPrefix(s, prefix) {
			if idx := strings.Index(s, ": "); idx != -1 {
				s = s[idx+2:]
			}
			break
		}
	}
	if len(s) > 120 {
		s = s[:117] + "..."
	}
	return s
}
