package ezhttp

import (
	"net/http"

	"github.com/quic-go/quic-go/http3"
)

// h3RoundTripper returns an HTTP/3 (QUIC) transport.
// TLS fingerprinting is not available for HTTP/3 since QUIC uses
// its own TLS 1.3 handshake. Browser headers are still applied.
func h3RoundTripper() http.RoundTripper {
	return &http3.Transport{}
}
