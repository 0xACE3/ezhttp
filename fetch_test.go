package fetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"name": "alice"})
	}))
	defer srv.Close()

	client := Client{Base: srv.URL}
	var user struct{ Name string }
	if err := client.Get(context.Background(), "/users/1").JSON(&user); err != nil {
		t.Fatal(err)
	}
	if user.Name != "alice" {
		t.Fatalf("got %q, want alice", user.Name)
	}
}

func TestGetText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	}))
	defer srv.Close()

	client := Client{Base: srv.URL}
	text := client.Get(context.Background(), "/health").Text()
	if text != "hello" {
		t.Fatalf("got %q, want hello", text)
	}
}

func TestPost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("got method %s, want POST", r.Method)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		json.NewEncoder(w).Encode(map[string]string{"id": "1", "name": body["name"]})
	}))
	defer srv.Close()

	client := Client{Base: srv.URL}
	resp := client.Post(context.Background(), "/users", map[string]string{"name": "bob"})
	name := resp.Path("name").String()
	if name != "bob" {
		t.Fatalf("got %q, want bob", name)
	}
}

func TestPathTraversal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"quote":{"USD":{"price":42000.50}}}}`))
	}))
	defer srv.Close()

	client := Client{Base: srv.URL}
	resp := client.Get(context.Background(), "/api/v1/market/BTC")
	price := resp.Path("data", "quote", "USD", "price").Float()
	if price != 42000.50 {
		t.Fatalf("got %f, want 42000.50", price)
	}

	keys := resp.Path("data", "quote").Keys()
	if len(keys) != 1 || keys[0] != "USD" {
		t.Fatalf("got keys %v, want [USD]", keys)
	}
}

func TestThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`ENCRYPTED:{"ok":true}`))
	}))
	defer srv.Close()

	decrypt := func(b []byte) ([]byte, error) {
		return b[len("ENCRYPTED:"):], nil
	}

	client := Client{Base: srv.URL}
	var result struct{ OK bool }
	err := client.Get(context.Background(), "/api").Through(decrypt).JSON(&result)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatal("expected OK=true")
	}
}

func TestTo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"name": "alice", "age": 30})
	}))
	defer srv.Close()

	type User struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	client := Client{Base: srv.URL}
	user, err := To[User](client.Get(context.Background(), "/users/1"))
	if err != nil {
		t.Fatal(err)
	}
	if user.Name != "alice" || user.Age != 30 {
		t.Fatalf("got %+v", user)
	}
}

func TestHTMLParsing(t *testing.T) {
	html := `<html><body>
		<h1>Products</h1>
		<div class="product-card">
			<span class="name">Widget</span>
			<span class="price">$9.99</span>
			<span class="sku" data-id="W001">SKU</span>
		</div>
		<div class="product-card">
			<span class="name">Gadget</span>
			<span class="price">$19.99</span>
			<span class="sku" data-id="G002">SKU</span>
		</div>
	</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer srv.Close()

	client := Client{Base: srv.URL}
	doc, err := client.Get(context.Background(), "/products").HTML()
	if err != nil {
		t.Fatal(err)
	}

	title := doc.Find("h1").Text()
	if title != "Products" {
		t.Fatalf("got %q, want Products", title)
	}

	names := doc.FindAll(".product-card .name").Text()
	if len(names) != 2 || names[0] != "Widget" || names[1] != "Gadget" {
		t.Fatalf("got names %v", names)
	}

	type Product struct {
		Name  string `css:".name"`
		Price string `css:".price"`
		SKU   string `css:".sku" attr:"data-id"`
	}
	var products []Product
	doc.FindAll(".product-card").Decode(&products)
	if len(products) != 2 {
		t.Fatalf("got %d products", len(products))
	}
	if products[0].Name != "Widget" || products[0].Price != "$9.99" || products[0].SKU != "W001" {
		t.Fatalf("got %+v", products[0])
	}
	if products[1].Name != "Gadget" || products[1].SKU != "G002" {
		t.Fatalf("got %+v", products[1])
	}
}

func TestErrorHandling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(429)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	client := Client{Base: srv.URL}
	resp := client.Get(context.Background(), "/api")
	if resp.Status != 429 {
		t.Fatalf("got status %d, want 429", resp.Status)
	}
	if resp.Err() == nil {
		t.Fatal("expected error")
	}

	re, ok := resp.Err().(*ResponseError)
	if !ok {
		t.Fatal("expected *ResponseError")
	}
	if re.RetryAfter != 5*time.Second {
		t.Fatalf("got retry-after %v, want 5s", re.RetryAfter)
	}
}

func TestDynamicHeaders(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := Client{
		Base: srv.URL,
		Headers: func() Headers {
			return Headers{"X-Custom": "dynamic-value"}
		},
	}
	client.Get(context.Background(), "/test")
	if gotHeader != "dynamic-value" {
		t.Fatalf("got header %q, want dynamic-value", gotHeader)
	}
}

func TestFullURLOverridesBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("from-server"))
	}))
	defer srv.Close()

	client := Client{Base: "https://will-not-be-used.example.com"}
	text := client.Get(context.Background(), srv.URL+"/data").Text()
	if text != "from-server" {
		t.Fatalf("got %q", text)
	}
}

func TestQuery(t *testing.T) {
	q := Query{
		"symbol": "BTC",
		"limit":  "100",
	}
	encoded := q.Encode()
	if encoded == "" || encoded[0] != '?' {
		t.Fatalf("expected query starting with ?, got %q", encoded)
	}
}

func TestQueryList(t *testing.T) {
	ql := QueryList{
		{"symbol", "BTC"},
		{"symbol", "ETH"},
		{"interval", "1h"},
	}
	encoded := ql.Encode()
	if encoded == "" || encoded[0] != '?' {
		t.Fatalf("expected query starting with ?, got %q", encoded)
	}
}

func TestWith(t *testing.T) {
	client := Client{Base: "https://api.example.com", Timeout: 10 * time.Second}
	sec := client.With(Override{Base: "https://efts.sec.gov"})
	if sec.Base != "https://efts.sec.gov" {
		t.Fatalf("got base %q", sec.Base)
	}
	if sec.Timeout != 10*time.Second {
		t.Fatalf("timeout not inherited: %v", sec.Timeout)
	}
}

func TestChainThrough(t *testing.T) {
	upper := func(b []byte) ([]byte, error) {
		out := make([]byte, len(b))
		for i, c := range b {
			if c >= 'a' && c <= 'z' {
				out[i] = c - 32
			} else {
				out[i] = c
			}
		}
		return out, nil
	}
	double := func(b []byte) ([]byte, error) {
		return append(b, b...), nil
	}

	combined := Chain(upper, double)
	result, err := combined([]byte("hi"))
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != "HIHI" {
		t.Fatalf("got %q", string(result))
	}
}
