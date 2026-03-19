package ezhttp

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"sync"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
)

// Browser configures TLS fingerprinting and browser header impersonation.
// Use the pre-built profiles: Chrome, Firefox, Safari, Edge.
type Browser struct {
	name        string
	clientHello utls.ClientHelloID
	userAgents  []string
	headers     func(ua string) Headers

	mu    sync.Mutex
	uaIdx int
}

// rotateUA returns the next user-agent in round-robin.
func (b *Browser) rotateUA() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ua := b.userAgents[b.uaIdx%len(b.userAgents)]
	b.uaIdx++
	return ua
}

// browserHeaders returns the full set of default browser headers with a rotated UA.
func (b *Browser) browserHeaders() Headers {
	ua := b.rotateUA()
	return b.headers(ua)
}

// --- Pre-built profiles ---

// Chrome impersonates Google Chrome (TLS + headers + UA rotation).
var Chrome = &Browser{
	name:        "chrome",
	clientHello: utls.HelloChrome_Auto,
	userAgents: []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	},
	headers: func(ua string) Headers {
		return Headers{
			"User-Agent":         ua,
			"Accept":             "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
			"Accept-Language":    "en-US,en;q=0.9",
			"Accept-Encoding":    "gzip, deflate, br",
			"Sec-Ch-Ua":          `"Not_A Brand";v="8", "Chromium";v="131", "Google Chrome";v="131"`,
			"Sec-Ch-Ua-Mobile":   "?0",
			"Sec-Ch-Ua-Platform": `"Windows"`,
			"Sec-Fetch-Dest":     "document",
			"Sec-Fetch-Mode":     "navigate",
			"Sec-Fetch-Site":     "none",
			"Sec-Fetch-User":     "?1",
			"Upgrade-Insecure-Requests": "1",
		}
	},
}

// Firefox impersonates Mozilla Firefox (TLS + headers + UA rotation).
var Firefox = &Browser{
	name:        "firefox",
	clientHello: utls.HelloFirefox_Auto,
	userAgents: []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:132.0) Gecko/20100101 Firefox/132.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14.7; rv:133.0) Gecko/20100101 Firefox/133.0",
		"Mozilla/5.0 (X11; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0",
	},
	headers: func(ua string) Headers {
		return Headers{
			"User-Agent":      ua,
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
			"Accept-Language":  "en-US,en;q=0.5",
			"Accept-Encoding":  "gzip, deflate, br",
			"Sec-Fetch-Dest":   "document",
			"Sec-Fetch-Mode":   "navigate",
			"Sec-Fetch-Site":   "none",
			"Sec-Fetch-User":   "?1",
			"Upgrade-Insecure-Requests": "1",
		}
	},
}

// Safari impersonates Apple Safari (TLS + headers + UA rotation).
var Safari = &Browser{
	name:        "safari",
	clientHello: utls.HelloSafari_Auto,
	userAgents: []string{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_6) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.6 Safari/605.1.15",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
	},
	headers: func(ua string) Headers {
		return Headers{
			"User-Agent":      ua,
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			"Accept-Language":  "en-US,en;q=0.9",
			"Accept-Encoding":  "gzip, deflate, br",
		}
	},
}

// Edge impersonates Microsoft Edge (TLS + headers + UA rotation).
var Edge = &Browser{
	name:        "edge",
	clientHello: utls.HelloEdge_Auto,
	userAgents: []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0",
	},
	headers: func(ua string) Headers {
		return Headers{
			"User-Agent":         ua,
			"Accept":             "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
			"Accept-Language":    "en-US,en;q=0.9",
			"Accept-Encoding":    "gzip, deflate, br",
			"Sec-Ch-Ua":          `"Not_A Brand";v="8", "Chromium";v="131", "Microsoft Edge";v="131"`,
			"Sec-Ch-Ua-Mobile":   "?0",
			"Sec-Ch-Ua-Platform": `"Windows"`,
			"Sec-Fetch-Dest":     "document",
			"Sec-Fetch-Mode":     "navigate",
			"Sec-Fetch-Site":     "none",
			"Sec-Fetch-User":     "?1",
			"Upgrade-Insecure-Requests": "1",
		}
	},
}

// RandomBrowser returns a random browser profile.
func RandomBrowser() *Browser {
	profiles := []*Browser{Chrome, Firefox, Safari, Edge}
	return profiles[rand.IntN(len(profiles))]
}

// --- TLS fingerprint transport ---

// utlsDial performs a TLS handshake via utls with the given ALPN protocols.
func utlsDial(ctx context.Context, b *Browser, proxyURL *url.URL, network, addr string, alpn []string) (net.Conn, error) {
	conn, err := dialWithProxy(ctx, network, addr, proxyURL)
	if err != nil {
		return nil, err
	}
	host, _, _ := net.SplitHostPort(addr)

	spec, err := utls.UTLSIdToSpec(b.clientHello)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("utls spec: %w", err)
	}
	for _, ext := range spec.Extensions {
		if a, ok := ext.(*utls.ALPNExtension); ok {
			a.AlpnProtocols = alpn
			break
		}
	}

	tlsConn := utls.UClient(conn, &utls.Config{ServerName: host}, utls.HelloCustom)
	if err := tlsConn.ApplyPreset(&spec); err != nil {
		conn.Close()
		return nil, fmt.Errorf("utls apply preset: %w", err)
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return tlsConn, nil
}

// fingerprintH1Transport creates an HTTP/1.1-only transport with TLS fingerprinting.
func fingerprintH1Transport(b *Browser, proxyStr string) *http.Transport {
	var proxyURL *url.URL
	if proxyStr != "" {
		proxyURL, _ = url.Parse(proxyStr)
	}

	t := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return utlsDial(ctx, b, proxyURL, network, addr, []string{"http/1.1"})
		},
	}

	if proxyURL != nil {
		t.Proxy = http.ProxyURL(proxyURL)
	}

	return t
}

// fingerprintH2Transport creates an HTTP/2 transport with TLS fingerprinting.
// Uses golang.org/x/net/http2.Transport so h2 framing is handled correctly.
func fingerprintH2Transport(b *Browser, proxyStr string) *http2.Transport {
	var proxyURL *url.URL
	if proxyStr != "" {
		proxyURL, _ = url.Parse(proxyStr)
	}

	return &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return utlsDial(ctx, b, proxyURL, network, addr, []string{"h2", "http/1.1"})
		},
	}
}

// dialWithProxy dials through the configured proxy, or directly if none.
func dialWithProxy(ctx context.Context, network, addr string, proxyURL *url.URL) (net.Conn, error) {
	dialer := &net.Dialer{}

	if proxyURL == nil {
		return dialer.DialContext(ctx, network, addr)
	}

	switch proxyURL.Scheme {
	case "socks5", "socks5h":
		socksDialer, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("socks5 proxy: %w", err)
		}
		if cd, ok := socksDialer.(proxy.ContextDialer); ok {
			return cd.DialContext(ctx, network, addr)
		}
		return socksDialer.Dial(network, addr)

	case "http", "https":
		return dialHTTPConnect(ctx, proxyURL, addr)

	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", proxyURL.Scheme)
	}
}

// dialHTTPConnect establishes a CONNECT tunnel through an HTTP proxy.
func dialHTTPConnect(ctx context.Context, proxyURL *url.URL, targetAddr string) (net.Conn, error) {
	proxyAddr := proxyURL.Host
	if proxyURL.Port() == "" {
		proxyAddr = net.JoinHostPort(proxyURL.Hostname(), "80")
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("dial proxy: %w", err)
	}

	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", targetAddr, targetAddr)
	if proxyURL.User != nil {
		connectReq += "Proxy-Authorization: Basic " + BasicAuth(
			proxyURL.User.Username(),
			func() string { p, _ := proxyURL.User.Password(); return p }(),
		) + "\r\n"
	}
	connectReq += "\r\n"

	if _, err := conn.Write([]byte(connectReq)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("proxy CONNECT write: %w", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "CONNECT"})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("proxy CONNECT response: %w", err)
	}
	if resp.StatusCode != 200 {
		conn.Close()
		return nil, fmt.Errorf("proxy CONNECT returned %d", resp.StatusCode)
	}

	return conn, nil
}
