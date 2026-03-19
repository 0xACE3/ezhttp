package tests

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xACE3/ezhttp"
)

// ===========================
// 1. BASIC GET + JSON
// ===========================

func TestGetJSON_Httpbin(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}
	ctx := context.Background()

	type IP struct {
		Origin string `json:"origin"`
	}
	var ip IP
	err := client.Get(ctx, "/ip").JSON(&ip)
	if err != nil {
		t.Fatal(err)
	}
	if ip.Origin == "" {
		t.Fatal("expected non-empty origin IP")
	}
	t.Logf("My IP: %s", ip.Origin)
}

func TestGetText_Httpbin(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}
	ctx := context.Background()

	text := client.Get(ctx, "/robots.txt").Text()
	if text == "" {
		t.Fatal("expected non-empty robots.txt")
	}
	t.Logf("robots.txt length: %d", len(text))
}

// ===========================
// 2. POST JSON
// ===========================

func TestPostJSON_Httpbin(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}
	ctx := context.Background()

	payload := map[string]string{"name": "fetch", "lang": "go"}
	resp := client.Post(ctx, "/post", payload)
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}
	if resp.Status != 200 {
		t.Fatalf("got status %d", resp.Status)
	}

	name := resp.Path("json", "name").String()
	lang := resp.Path("json", "lang").String()
	if name != "fetch" {
		t.Fatalf("expected name=fetch, got %q", name)
	}
	if lang != "go" {
		t.Fatalf("expected lang=go, got %q", lang)
	}
	t.Logf("POST echoed: name=%s, lang=%s", name, lang)
}

// ===========================
// 3. PUT / PATCH / DELETE / HEAD
// ===========================

func TestPut_Httpbin(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}
	resp := client.Put(context.Background(), "/put", map[string]string{"key": "val"})
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}
	if resp.Path("json", "key").String() != "val" {
		t.Fatal("PUT echo mismatch")
	}
}

func TestPatch_Httpbin(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}
	resp := client.Patch(context.Background(), "/patch", map[string]string{"patched": "true"})
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}
	if resp.Path("json", "patched").String() != "true" {
		t.Fatal("PATCH echo mismatch")
	}
}

func TestDelete_Httpbin(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}
	resp := client.Delete(context.Background(), "/delete")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}
	if resp.Status != 200 {
		t.Fatalf("got status %d", resp.Status)
	}
}

func TestHead_Httpbin(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}
	resp := client.Head(context.Background(), "/get")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}
	if resp.Status != 200 {
		t.Fatalf("got status %d", resp.Status)
	}
	if len(resp.Body) != 0 {
		t.Fatalf("expected empty body for HEAD, got %d bytes", len(resp.Body))
	}
}

// ===========================
// 4. DYNAMIC HEADERS
// ===========================

func TestDynamicHeaders_Httpbin(t *testing.T) {
	client := ezhttp.Client{
		Base:    "https://httpbin.org",
		Timeout: 15 * time.Second,
		Headers: func() ezhttp.Headers {
			return ezhttp.Headers{
				"X-Custom-Token": "bearer-abc123",
				"User-Agent":     "fetch-test/1.0",
			}
		},
	}

	resp := client.Get(context.Background(), "/headers")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}
	ua := resp.Path("headers", "User-Agent").String()
	token := resp.Path("headers", "X-Custom-Token").String()
	if ua != "fetch-test/1.0" {
		t.Fatalf("expected custom UA, got %q", ua)
	}
	if token != "bearer-abc123" {
		t.Fatalf("expected token header, got %q", token)
	}
	t.Logf("Headers confirmed: UA=%s, Token=%s", ua, token)
}

// ===========================
// 5. JSON PATH TRAVERSAL (real API)
// ===========================

func TestPathTraversal_GithubAPI(t *testing.T) {
	client := ezhttp.Client{Timeout: 15 * time.Second}

	resp := client.Get(context.Background(), "https://api.github.com/repos/golang/go")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	name := resp.Path("full_name").String()
	if name != "golang/go" {
		t.Fatalf("expected golang/go, got %q", name)
	}

	lang := resp.Path("language").String()
	if lang != "Go" {
		t.Fatalf("expected Go, got %q", lang)
	}

	stars := resp.Path("stargazers_count").Int()
	if stars < 100000 {
		t.Fatalf("expected >100k stars, got %d", stars)
	}

	owner := resp.Path("owner", "login").String()
	if owner != "golang" {
		t.Fatalf("expected owner golang, got %q", owner)
	}

	t.Logf("golang/go: lang=%s, stars=%d, owner=%s", lang, stars, owner)
}

func TestPathArray_GithubAPI(t *testing.T) {
	client := ezhttp.Client{Timeout: 15 * time.Second}

	resp := client.Get(context.Background(), "https://api.github.com/repos/golang/go/tags?per_page=3")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	// Top-level array — iterate via @this
	var names []string
	resp.Path("@this").Each(func(i int, v ezhttp.Value) {
		names = append(names, v.Path("name").String())
	})
	if len(names) == 0 {
		t.Fatal("expected at least one tag")
	}
	t.Logf("Tags: %v", names)
}

// ===========================
// 6. JSON Keys + Decode subsection
// ===========================

func TestPathKeys_Httpbin(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}

	resp := client.Get(context.Background(), "/headers")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	keys := resp.Path("headers").Keys()
	if len(keys) == 0 {
		t.Fatal("expected header keys")
	}
	t.Logf("Header keys: %v", keys)

	type HeadersResult struct {
		Host string `json:"Host"`
	}
	var h HeadersResult
	if err := resp.Path("headers").Decode(&h); err != nil {
		t.Fatal(err)
	}
	if h.Host != "httpbin.org" {
		t.Fatalf("expected host httpbin.org, got %q", h.Host)
	}
}

// ===========================
// 7. GENERIC To[T]
// ===========================

func TestToGeneric_Httpbin(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}

	type IPResult struct {
		Origin string `json:"origin"`
	}
	ip, err := ezhttp.To[IPResult](client.Get(context.Background(), "/ip"))
	if err != nil {
		t.Fatal(err)
	}
	if ip.Origin == "" {
		t.Fatal("expected non-empty IP")
	}
	t.Logf("To[IPResult]: %s", ip.Origin)
}

func TestToGeneric_WithThrough(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}

	type GetResult struct {
		URL string `json:"url"`
	}
	result, err := ezhttp.To[GetResult](
		client.Get(context.Background(), "/get").Through(func(b []byte) ([]byte, error) {
			return b, nil // identity transform — verifies Through chains with To
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.URL, "httpbin.org/get") {
		t.Fatalf("unexpected URL: %q", result.URL)
	}
}

// ===========================
// 8. THROUGH PIPELINE
// ===========================

func TestThrough_ChainTransforms(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}

	strip := func(b []byte) ([]byte, error) {
		return []byte(strings.TrimSpace(string(b))), nil
	}
	addMarker := func(b []byte) ([]byte, error) {
		return append([]byte("PROCESSED:"), b...), nil
	}

	text := client.Get(context.Background(), "/robots.txt").
		Through(ezhttp.Chain(strip, addMarker)).
		Text()

	if !strings.HasPrefix(text, "PROCESSED:") {
		t.Fatalf("chain transform failed, got prefix: %q", text[:min(30, len(text))])
	}
	t.Logf("Chain result starts with: %s", text[:min(40, len(text))])
}

func TestThrough_DecodeBase64_Httpbin(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}

	// /base64/{encoded} returns the decoded value
	resp := client.Get(context.Background(), "/base64/aGVsbG8gZmV0Y2g=")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}
	text := resp.Text()
	if text != "hello fetch" {
		t.Fatalf("expected 'hello fetch', got %q", text)
	}
	t.Logf("Base64 decoded: %s", text)
}

// ===========================
// 9. HTML SCRAPING
// ===========================

func TestHTML_ExampleDotCom(t *testing.T) {
	client := ezhttp.Client{Timeout: 15 * time.Second}

	doc, err := client.Get(context.Background(), "https://example.com").HTML()
	if err != nil {
		t.Fatal(err)
	}

	title := doc.Find("h1").Text()
	if title != "Example Domain" {
		t.Fatalf("expected 'Example Domain', got %q", title)
	}

	link := doc.Find("a").Attr("href")
	if link == "" {
		t.Fatal("expected a link on example.com")
	}

	if !doc.Find("h1").Exists() {
		t.Fatal("h1 should exist")
	}
	if doc.Find(".nonexistent-class").Exists() {
		t.Fatal("nonexistent class should not exist")
	}

	t.Logf("example.com: title=%q, link=%q", title, link)
}

func TestHTML_HackerNews(t *testing.T) {
	client := ezhttp.Client{
		Timeout: 15 * time.Second,
		Headers: func() ezhttp.Headers {
			return ezhttp.Headers{
				"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0",
			}
		},
	}

	doc, err := client.Get(context.Background(), "https://news.ycombinator.com").HTML()
	if err != nil {
		t.Fatal(err)
	}

	titles := doc.FindAll(".titleline > a").Text()
	if len(titles) < 10 {
		t.Fatalf("expected at least 10 stories, got %d", len(titles))
	}

	links := doc.FindAll(".titleline > a").Attr("href")
	if len(links) == 0 {
		t.Fatal("expected story links")
	}

	t.Logf("HN stories: %d", len(titles))
	for i := range min(3, len(titles)) {
		t.Logf("  %d. %s -> %s", i+1, titles[i], links[i])
	}
}

func TestHTML_CSSStructDecode(t *testing.T) {
	client := ezhttp.Client{
		Timeout: 15 * time.Second,
		Headers: func() ezhttp.Headers {
			return ezhttp.Headers{
				"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0",
			}
		},
	}

	type Story struct {
		Title string `css:".titleline > a"`
		Site  string `css:".sitestr"`
	}

	doc, err := client.Get(context.Background(), "https://news.ycombinator.com").HTML()
	if err != nil {
		t.Fatal(err)
	}

	var stories []Story
	doc.FindAll(".athing").Decode(&stories)

	if len(stories) == 0 {
		t.Fatal("expected decoded stories")
	}

	hasTitle := false
	for _, s := range stories {
		if s.Title != "" {
			hasTitle = true
			break
		}
	}
	if !hasTitle {
		t.Fatal("no stories had titles after decode")
	}

	t.Logf("Decoded %d stories", len(stories))
	for i := range min(3, len(stories)) {
		t.Logf("  %d. %q (site: %s)", i+1, stories[i].Title, stories[i].Site)
	}
}

func TestHTML_Navigation(t *testing.T) {
	client := ezhttp.Client{Timeout: 15 * time.Second}

	doc, err := client.Get(context.Background(), "https://example.com").HTML()
	if err != nil {
		t.Fatal(err)
	}

	body := doc.Find("body")
	if !body.Exists() {
		t.Fatal("body should exist")
	}

	children := body.Children()
	if children.Len() == 0 {
		t.Fatal("body should have children")
	}
	t.Logf("body has %d children", children.Len())

	first := children.First()
	if !first.Exists() {
		t.Fatal("first child should exist")
	}
	txt := first.Text()
	t.Logf("First child text: %q", txt[:min(50, len(txt))])
}

// ===========================
// 10. ERROR HANDLING
// ===========================

func TestError_404(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}

	resp := client.Get(context.Background(), "/status/404")
	if resp.Status != 404 {
		t.Fatalf("expected 404, got %d", resp.Status)
	}
	if resp.Err() == nil {
		t.Fatal("expected error for 404")
	}
	re, ok := resp.Err().(*ezhttp.ResponseError)
	if !ok {
		t.Fatalf("expected *ResponseError, got %T", resp.Err())
	}
	if re.Status != 404 {
		t.Fatalf("ResponseError status: got %d, want 404", re.Status)
	}
	t.Logf("404 error: %v", re)
}

func TestError_500(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}

	resp := client.Get(context.Background(), "/status/500")
	if resp.Status != 500 {
		t.Fatalf("expected 500, got %d", resp.Status)
	}
	re, ok := resp.Err().(*ezhttp.ResponseError)
	if !ok {
		t.Fatal("expected *ResponseError")
	}
	if re.Status != 500 {
		t.Fatalf("got status %d", re.Status)
	}
}

func TestError_JSONFlowsError(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}

	var result map[string]any
	err := client.Get(context.Background(), "/status/403").JSON(&result)
	if err == nil {
		t.Fatal("expected error")
	}
	re, ok := err.(*ezhttp.ResponseError)
	if !ok {
		t.Fatalf("expected *ResponseError, got %T: %v", err, err)
	}
	if re.Status != 403 {
		t.Fatalf("got status %d", re.Status)
	}
}

func TestError_Timeout(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 2 * time.Second}

	resp := client.Get(context.Background(), "/delay/10")
	if resp.Err() == nil {
		t.Fatal("expected timeout error")
	}
	t.Logf("Timeout error: %v", resp.Err())
}

func TestError_ContextCancel(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 30 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	resp := client.Get(ctx, "/delay/10")
	if resp.Err() == nil {
		t.Fatal("expected context cancel error")
	}
	t.Logf("Cancel error: %v", resp.Err())
}

// ===========================
// 11. FULL URL OVERRIDES BASE
// ===========================

func TestFullURLOverridesBase(t *testing.T) {
	client := ezhttp.Client{Base: "https://will-not-be-used.invalid", Timeout: 15 * time.Second}

	resp := client.Get(context.Background(), "https://httpbin.org/ip")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}
	if resp.Path("origin").String() == "" {
		t.Fatal("expected IP from httpbin")
	}
}

// ===========================
// 12. QUERY BUILDERS
// ===========================

func TestQuery_Httpbin(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}

	q := ezhttp.Query{
		"symbol": "BTC",
		"limit":  "5",
		"sort":   "desc",
	}
	resp := client.Get(context.Background(), "/get"+q.Encode())
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	sym := resp.Path("args", "symbol").String()
	lim := resp.Path("args", "limit").String()
	srt := resp.Path("args", "sort").String()
	if sym != "BTC" || lim != "5" || srt != "desc" {
		t.Fatalf("query args mismatch: sym=%s, lim=%s, sort=%s", sym, lim, srt)
	}
	t.Logf("Query args: symbol=%s, limit=%s, sort=%s", sym, lim, srt)
}

func TestQueryList_Httpbin(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}

	ql := ezhttp.QueryList{
		{"tag", "crypto"},
		{"tag", "defi"},
		{"tag", "nft"},
		{"limit", "10"},
	}
	resp := client.Get(context.Background(), "/get"+ql.Encode())
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	tags := resp.Path("args", "tag").String()
	if tags == "" {
		t.Fatal("expected tag args")
	}
	t.Logf("QueryList tags: %s", tags)
}

// ===========================
// 13. WITH (DERIVED CLIENT)
// ===========================

func TestWith_DerivedClient(t *testing.T) {
	base := ezhttp.Client{
		Base:    "https://httpbin.org",
		Timeout: 15 * time.Second,
	}

	derived := base.With(ezhttp.Override{Base: "https://api.github.com"})

	resp := derived.Get(context.Background(), "/repos/golang/go")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}
	if resp.Path("full_name").String() != "golang/go" {
		t.Fatal("derived client didn't use new base")
	}

	resp2 := base.Get(context.Background(), "/ip")
	if resp2.Err() != nil {
		t.Fatal(resp2.Err())
	}
	if resp2.Path("origin").String() == "" {
		t.Fatal("original client broken")
	}

	if derived.Timeout != 15*time.Second {
		t.Fatalf("timeout not inherited: %v", derived.Timeout)
	}
}

// ===========================
// 14. POLLING
// ===========================

func TestPoll_Single(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}

	stream := client.Poll(context.Background(), ezhttp.PollConfig{
		Path:     "/uuid",
		Interval: 2 * time.Second,
	})

	var uuids []string
	timeout := time.After(7 * time.Second)
loop:
	for {
		select {
		case resp, ok := <-stream.Values:
			if !ok {
				break loop
			}
			if resp.Err() != nil {
				t.Fatal(resp.Err())
			}
			uuids = append(uuids, resp.Path("uuid").String())
			t.Logf("Poll UUID: %s", uuids[len(uuids)-1])
			if len(uuids) >= 3 {
				stream.Stop()
				break loop
			}
		case <-timeout:
			stream.Stop()
			break loop
		}
	}
	if len(uuids) < 2 {
		t.Fatalf("expected at least 2 poll responses, got %d", len(uuids))
	}
	seen := map[string]bool{}
	for _, u := range uuids {
		if seen[u] {
			t.Fatalf("duplicate UUID: %s", u)
		}
		seen[u] = true
	}
	t.Logf("Polled %d unique UUIDs", len(uuids))
}

func TestPollMany_Multiple(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}

	stream := client.PollMany(context.Background(), ezhttp.PollConfig{
		Paths: []string{
			"/get?endpoint=alpha",
			"/get?endpoint=beta",
			"/get?endpoint=gamma",
		},
		Interval: 3 * time.Second,
	})

	seen := map[string]int{}
	timeout := time.After(8 * time.Second)
loop:
	for {
		select {
		case resp, ok := <-stream.Values:
			if !ok {
				break loop
			}
			if resp.Err() != nil {
				continue
			}
			ep := resp.Path("args", "endpoint").String()
			seen[ep]++
			t.Logf("PollMany: endpoint=%s", ep)
		case <-timeout:
			stream.Stop()
			break loop
		}
	}
	if len(seen) < 3 {
		t.Fatalf("expected 3 endpoints, saw %d: %v", len(seen), seen)
	}
	t.Logf("PollMany endpoints hit: %v", seen)
}

func TestPoll_StopWhen(t *testing.T) {
	client := ezhttp.Client{Base: "https://httpbin.org", Timeout: 15 * time.Second}

	count := 0
	stream := client.Poll(context.Background(), ezhttp.PollConfig{
		Path:     "/uuid",
		Interval: 1 * time.Second,
		StopWhen: func(v ezhttp.Value) bool {
			count++
			return count >= 2
		},
	})

	var responses int
	timeout := time.After(10 * time.Second)
loop:
	for {
		select {
		case _, ok := <-stream.Values:
			if !ok {
				break loop
			}
			responses++
		case <-timeout:
			stream.Stop()
			break loop
		}
	}
	if responses < 2 {
		t.Fatalf("expected at least 2 responses before stop, got %d", responses)
	}
	t.Logf("StopWhen triggered after %d responses", responses)
}

// ===========================
// 15. RESPONSE METADATA
// ===========================

func TestResponse_Metadata(t *testing.T) {
	client := ezhttp.Client{
		Base:    "https://httpbin.org",
		Timeout: 15 * time.Second,
		Headers: func() ezhttp.Headers {
			return ezhttp.Headers{"User-Agent": "fetch-integration-test"}
		},
	}

	resp := client.Get(context.Background(), "/get")
	if resp.Err() != nil {
		t.Fatal(resp.Err())
	}

	if resp.Status != 200 {
		t.Fatalf("status: %d", resp.Status)
	}
	if resp.Headers["Content-Type"] == "" {
		t.Fatal("expected Content-Type header")
	}
	t.Logf("Content-Type: %s", resp.Headers["Content-Type"])

	if !strings.Contains(resp.RequestURL, "httpbin.org/get") {
		t.Fatalf("unexpected RequestURL: %s", resp.RequestURL)
	}
	if resp.RequestHeaders["User-Agent"] != "fetch-integration-test" {
		t.Fatalf("RequestHeaders UA: %s", resp.RequestHeaders["User-Agent"])
	}
	if len(resp.Bytes()) == 0 {
		t.Fatal("expected non-empty bytes")
	}
}

// ===========================
// 16. SAVE TO FILE
// ===========================

func TestSave_Httpbin(t *testing.T) {
	client := ezhttp.Client{Timeout: 15 * time.Second}

	path := t.TempDir() + "/test-response.json"
	err := client.Get(context.Background(), "https://httpbin.org/uuid").Save(path)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("saved file is empty")
	}
	if !strings.Contains(string(data), "uuid") {
		t.Fatalf("saved file doesn't contain uuid: %s", string(data))
	}
	t.Logf("Saved %d bytes to %s", len(data), path)
}

// ===========================
// 17. REAL WORLD: COINGECKO
// ===========================

func TestRealWorld_CoinGecko(t *testing.T) {
	client := ezhttp.Client{
		Base:    "https://api.coingecko.com/api/v3",
		Timeout: 15 * time.Second,
		Headers: func() ezhttp.Headers {
			return ezhttp.Headers{"Accept": "application/json"}
		},
	}

	resp := client.Get(context.Background(), "/simple/price"+ezhttp.Query{
		"ids":           "bitcoin,ethereum",
		"vs_currencies": "usd",
	}.Encode())

	if resp.Err() != nil {
		t.Skipf("CoinGecko rate limited or down: %v", resp.Err())
	}

	btcPrice := resp.Path("bitcoin", "usd").Float()
	ethPrice := resp.Path("ethereum", "usd").Float()

	if btcPrice <= 0 {
		t.Fatalf("expected positive BTC price, got %f", btcPrice)
	}
	if ethPrice <= 0 {
		t.Fatalf("expected positive ETH price, got %f", ethPrice)
	}
	t.Logf("BTC: $%.2f, ETH: $%.2f", btcPrice, ethPrice)
}

// ===========================
// 18. REAL WORLD: DEXSCREENER
// ===========================

func TestRealWorld_DexScreener(t *testing.T) {
	client := ezhttp.Client{
		Base:    "https://api.dexscreener.com",
		Timeout: 15 * time.Second,
	}

	resp := client.Get(context.Background(), "/latest/dex/tokens/So11111111111111111111111111111111111111112")
	if resp.Err() != nil {
		t.Skipf("DexScreener unavailable: %v", resp.Err())
	}

	pairs := resp.Path("pairs")
	if !pairs.Exists() {
		t.Fatal("expected pairs in response")
	}

	var count int
	pairs.Each(func(i int, v ezhttp.Value) {
		count++
		if i == 0 {
			t.Logf("First pair: %s, price: %s, dex: %s",
				v.Path("baseToken", "symbol").String(),
				v.Path("priceUsd").String(),
				v.Path("dexId").String(),
			)
		}
	})
	if count == 0 {
		t.Fatal("expected at least one pair")
	}
	t.Logf("DexScreener returned %d pairs", count)
}

// ===========================
// 19. REAL WORLD: GITHUB SEARCH
// ===========================

func TestRealWorld_GithubSearch(t *testing.T) {
	client := ezhttp.Client{
		Timeout: 15 * time.Second,
		Headers: func() ezhttp.Headers {
			return ezhttp.Headers{"Accept": "application/vnd.github.v3+json"}
		},
	}

	resp := client.Get(context.Background(), "https://api.github.com/search/repositories"+ezhttp.Query{
		"q":        "language:go stars:>50000",
		"sort":     "stars",
		"per_page": "5",
	}.Encode())

	if resp.Err() != nil {
		t.Skipf("GitHub API unavailable: %v", resp.Err())
	}

	total := resp.Path("total_count").Int()
	if total == 0 {
		t.Fatal("expected search results")
	}

	resp.Path("items").Each(func(i int, v ezhttp.Value) {
		t.Logf("  %s (%d stars)", v.Path("full_name").String(), v.Path("stargazers_count").Int())
	})
	t.Logf("GitHub search: %d total results", total)
}
