package ezhttp

import (
	"crypto/tls"
	"net/http"

	"golang.org/x/net/http2"
)

// Version selects the HTTP protocol version.
type Version int

const (
	// Auto negotiates the best protocol. With Browser fingerprinting,
	// defaults to HTTP/2 (matching real browser behavior). Without
	// fingerprinting, uses Go's standard h1/h2 negotiation.
	Auto Version = iota

	// HTTP1 forces HTTP/1.1 only.
	HTTP1

	// HTTP2 forces HTTP/2.
	HTTP2

	// HTTP3 forces HTTP/3 (QUIC). TLS fingerprinting is not available
	// for HTTP/3 since QUIC uses its own TLS 1.3 handshake.
	// Browser headers are still applied.
	HTTP3
)

// multiTransport routes HTTPS to h2 and plain HTTP to h1.
type multiTransport struct {
	https http.RoundTripper
	http  http.RoundTripper
}

func (m *multiTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "https" && m.https != nil {
		return m.https.RoundTrip(req)
	}
	return m.http.RoundTrip(req)
}

// standardH1Transport returns a basic http.Transport with h2 disabled.
func standardH1Transport(proxyStr string) *http.Transport {
	t := standardTransport(proxyStr)
	// Empty non-nil map disables automatic HTTP/2.
	t.TLSNextProto = make(map[string]func(authority string, c *tls.Conn) http.RoundTripper)
	return t
}

// standardH2Transport returns a standard http2.Transport (no fingerprinting).
func standardH2Transport() *http2.Transport {
	return &http2.Transport{}
}
