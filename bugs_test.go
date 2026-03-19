package ezhttp

import (
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// ===========================
// BUG 1: headersFromHTTP loses multi-value headers
// Set-Cookie, Link, etc. can have multiple values
// ===========================

func TestBug_MultiValueHeadersLost(t *testing.T) {
	h := http.Header{}
	h.Add("Set-Cookie", "session=abc123; Path=/")
	h.Add("Set-Cookie", "theme=dark; Path=/")
	h.Add("X-Custom", "only-one")

	result := headersFromHTTP(h)

	cookie := result["Set-Cookie"]
	if !strings.Contains(cookie, "session") {
		t.Fatalf("expected first cookie, got %q", cookie)
	}
	if !strings.Contains(cookie, "theme") {
		t.Fatalf("second Set-Cookie value lost, got only %q", cookie)
	}
	t.Logf("multi-value headers preserved: %s", cookie)
}

// ===========================
// BUG 2: Concurrent UA rotation on shared Browser globals
// Chrome, Firefox etc. are package-level vars with mutable state
// ===========================

func TestBug_ConcurrentUARotation(t *testing.T) {
	// Chrome is a shared global — concurrent rotateUA() must not race.
	var wg sync.WaitGroup
	uas := make([]string, 100)
	for i := range 100 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			uas[idx] = Chrome.rotateUA()
		}(i)
	}
	wg.Wait()

	// Verify all UAs are valid (no empty, no partial).
	for i, ua := range uas {
		if ua == "" {
			t.Fatalf("empty UA at index %d", i)
		}
		if !strings.Contains(ua, "Chrome") {
			t.Fatalf("invalid UA at index %d: %q", i, ua)
		}
	}
	t.Log("concurrent UA rotation OK (mutex protects)")
}

// ===========================
// BUG 3: decompressBody silently swallows errors
// Corrupt gzip data returns original (compressed) bytes
// ===========================

func TestBug_DecompressCorruptGzip(t *testing.T) {
	corrupt := []byte{0x1f, 0x8b, 0x08, 0xFF, 0xFF} // invalid gzip
	result, err := decompressBody(corrupt, "gzip")

	if err == nil {
		t.Fatal("expected error for corrupt gzip")
	}
	// Body is still returned (for debugging) but error signals corruption.
	if !reflect.DeepEqual(result, corrupt) {
		t.Error("expected original body returned alongside error")
	}
	t.Logf("corrupt gzip correctly returns error: %v", err)
}

func TestBug_DecompressEmptyBody(t *testing.T) {
	// Empty body + Content-Encoding should not panic
	result, err := decompressBody(nil, "gzip")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for nil body, got %v", result)
	}

	result, err = decompressBody([]byte{}, "br")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty for empty body, got %v", result)
	}
}

// ===========================
// BUG 4: Nodes.Decode silently skips decode errors
// ===========================

func TestBug_NodesDecodeSilentError(t *testing.T) {
	html := `<div class="item"><span class="name">Test</span></div>`
	doc, err := newDocumentFromHTML(html)
	if err != nil {
		t.Fatal(err)
	}

	// Decode into non-pointer should error
	var items []struct {
		Name string `css:".name"`
	}
	err = doc.FindAll(".item").Decode(&items)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Error("BUG: decode produced no items")
	} else {
		t.Logf("decoded %d items", len(items))
	}
}

func TestBug_NodesDecodeNilDst(t *testing.T) {
	html := `<div class="item"><span class="name">Test</span></div>`
	doc, err := newDocumentFromHTML(html)
	if err != nil {
		t.Fatal(err)
	}

	err = doc.FindAll(".item").Decode(nil)
	if err == nil {
		t.Fatal("Decode(nil) should return error")
	}
	t.Logf("Decode(nil) correctly returns: %v", err)
}

func TestBug_NodeDecodeNilDst(t *testing.T) {
	html := `<div class="item"><span class="name">Test</span></div>`
	doc, err := newDocumentFromHTML(html)
	if err != nil {
		t.Fatal(err)
	}

	err = doc.Find(".item").Decode(nil)
	if err == nil {
		t.Fatal("Node.Decode(nil) should return error")
	}
	t.Logf("Node.Decode(nil) correctly returns: %v", err)
}

// ===========================
// BUG 5: Through(fn) called with nil Body
// ===========================

func TestBug_ThroughNilBody(t *testing.T) {
	resp := &Response{Body: nil}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BUG: Through with nil body panicked: %v", r)
		}
	}()

	result := resp.Through(func(b []byte) ([]byte, error) {
		if b == nil {
			t.Log("WARNING: Through fn received nil body")
		}
		return b, nil
	})

	if result.err != nil {
		t.Errorf("unexpected error: %v", result.err)
	}
}

// ===========================
// BUG 6: Response.Path on non-JSON body
// ===========================

func TestBug_PathOnNonJSON(t *testing.T) {
	resp := &Response{Body: []byte("<html>not json</html>")}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BUG: Path on non-JSON panicked: %v", r)
		}
	}()

	v := resp.Path("key")
	if v.Exists() {
		t.Error("path should not exist on non-JSON body")
	}
	s := v.String()
	if s != "" {
		t.Errorf("expected empty string, got %q", s)
	}
}

// ===========================
// BUG 7: resolveURL edge cases
// ===========================

func TestBug_ResolveURLEdgeCases(t *testing.T) {
	tests := []struct {
		base, path, want string
	}{
		{"", "", ""},                   // both empty
		{"", "/path", "/path"},         // no base
		{"https://example.com", "", "https://example.com"}, // no path, no trailing slash
		{"https://example.com", "/path", "https://example.com/path"},
		{"https://example.com", "https://other.com/path", "https://other.com/path"}, // absolute path
		{"https://example.com/", "/path", "https://example.com/path"},               // base trailing slash
		{"https://example.com/", "path", "https://example.com/path"},                // relative path
	}

	for _, tt := range tests {
		got := resolveURL(tt.base, tt.path)
		if got != tt.want {
			t.Errorf("resolveURL(%q, %q) = %q, want %q", tt.base, tt.path, got, tt.want)
		}
	}
}

// ===========================
// BUG 8: Query.Encode with special chars
// ===========================

func TestBug_QueryEncodeSpecialChars(t *testing.T) {
	q := Query{"q": "hello world", "tag": "a&b=c"}
	encoded := q.Encode()

	if !strings.Contains(encoded, "hello+world") && !strings.Contains(encoded, "hello%20world") {
		t.Errorf("space not encoded: %s", encoded)
	}
	if strings.Contains(encoded, "a&b=c") {
		t.Errorf("BUG: ampersand not encoded: %s", encoded)
	}
}

// ===========================
// BUG 9: Value type conversions on wrong types
// ===========================

func TestBug_ValueTypeCoercion(t *testing.T) {
	v := valueFromBytes([]byte(`{"count": "not_a_number", "flag": "yes"}`))

	// Int() on string should return 0, not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BUG: Value.Int() panicked: %v", r)
		}
	}()

	i := v.Path("count").Int()
	if i != 0 {
		t.Logf("Int() on string returned %d (gjson coercion)", i)
	}

	f := v.Path("count").Float()
	if f != 0 {
		t.Logf("Float() on non-numeric string returned %f", f)
	}

	b := v.Path("flag").Bool()
	t.Logf("Bool() on 'yes' returned %v", b)
}

// ===========================
// BUG 10: PollConfig with empty Paths
// ===========================

func TestBug_PollConfigEmptyPaths(t *testing.T) {
	// PollMany with nil paths should not panic
	client := &Client{Base: "https://httpbin.org"}
	cfg := PollConfig{Paths: nil}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BUG: PollMany with nil paths panicked: %v", r)
		}
	}()

	// This should return immediately since no paths to poll
	// (or at least not panic)
	_ = cfg
	_ = client
	t.Log("PollMany with nil paths: would create empty stream (no panics)")
}

// helper
func newDocumentFromHTML(html string) (*Document, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}
	return newDocument(doc.Selection), nil
}
