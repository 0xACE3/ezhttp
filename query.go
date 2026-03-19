package ezhttp

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Headers is a map of HTTP headers. Used with Client.Headers func.
type Headers map[string]string

// Query is a map of query parameters with deterministic encoding.
type Query map[string]string

// Encode returns "?key=val&key2=val2" sorted by key. Returns "" if empty.
func (q Query) Encode() string {
	if len(q) == 0 {
		return ""
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	vals := url.Values{}
	for _, k := range keys {
		vals.Set(k, q[k])
	}
	return "?" + vals.Encode()
}

// QueryList supports repeated keys. Each entry is [key, value].
type QueryList [][2]string

// Encode returns "?key=v1&key=v2&..." preserving order. Returns "" if empty.
func (ql QueryList) Encode() string {
	if len(ql) == 0 {
		return ""
	}
	vals := url.Values{}
	for _, kv := range ql {
		vals.Add(kv[0], kv[1])
	}
	// url.Values.Encode sorts by key which is fine.
	return "?" + vals.Encode()
}

// BasicAuth returns the Authorization header value for HTTP Basic Auth.
func BasicAuth(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString(
		[]byte(fmt.Sprintf("%s:%s", user, pass)),
	)
}

// resolveURL resolves a path against a base URL.
// If path starts with http:// or https://, it's used as-is.
func resolveURL(base, path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if base == "" && path == "" {
		return ""
	}
	base = strings.TrimRight(base, "/")
	if path == "" {
		return base
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}
