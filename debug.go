package ezhttp

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// debugLog prints a single-line structured log to stderr when Debug is true.
//
// Format:
//
//	[ezhttp] GET 200 142ms 3.2kB https://httpbin.org/headers  h2 chrome
//	[ezhttp] POST 201  89ms  124B https://api.example.com/users
//	[ezhttp] GET 429 302ms   98B https://api.example.com/data  h2 chrome | retry 1/3 after=30s
//	[ezhttp] GET --- ERR          https://down.example.com  | err: connection refused
//	[ezhttp] WS  open             wss://stream.binance.com  chrome
//	[ezhttp] WS  closed           wss://stream.binance.com
func (c *Client) logRequest(method, url string, status int, latency time.Duration, bodySize int, err error) {
	if !c.Debug {
		return
	}

	var b strings.Builder
	b.WriteString("[ezhttp] ")

	// Method — left-padded to 6 for alignment.
	fmt.Fprintf(&b, "%-6s", method)

	// Status or ERR.
	if err != nil && status == 0 {
		b.WriteString("---  ")
	} else {
		fmt.Fprintf(&b, "%-5d", status)
	}

	// Latency.
	fmt.Fprintf(&b, "%-8s", formatDuration(latency))

	// Body size.
	fmt.Fprintf(&b, "%-8s", formatBytes(bodySize))

	// URL — truncated if too long.
	b.WriteString(truncURL(url, 72))

	// Tags: protocol + browser.
	tags := c.debugTags()
	if tags != "" {
		b.WriteString("  ")
		b.WriteString(tags)
	}

	// Error detail.
	if err != nil {
		b.WriteString("  | err: ")
		b.WriteString(compactErr(err))
	}

	fmt.Fprintln(os.Stderr, b.String())
}

func (c *Client) logRetry(method, url string, attempt, max int, wait time.Duration, status int) {
	if !c.Debug {
		return
	}
	var reason string
	switch {
	case status == 429:
		reason = "rate-limited"
	case status >= 500:
		reason = fmt.Sprintf("server-error(%d)", status)
	default:
		reason = "network-error"
	}
	fmt.Fprintf(os.Stderr, "[ezhttp] %-6s%-5s%-8s        %s  | retry %d/%d %s\n",
		method, "...", formatDuration(wait), truncURL(url, 72), attempt, max, reason)
}

func (c *Client) logWS(event, url string) {
	if !c.Debug {
		return
	}
	var b strings.Builder
	b.WriteString("[ezhttp] WS    ")
	fmt.Fprintf(&b, "%-21s", event)
	b.WriteString(truncURL(url, 72))

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

func truncURL(url string, max int) string {
	if len(url) <= max {
		return url
	}
	return url[:max-3] + "..."
}

func compactErr(err error) string {
	s := err.Error()
	// Strip common verbose prefixes.
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
