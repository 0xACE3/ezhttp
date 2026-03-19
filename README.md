# fetch

Minimal, fast HTTP client for Go built for scraping. Pure `net/http` with zero framework deps.

```
go get github.com/0xACE3/ezhttp@latest
```

```go
import fetch "github.com/0xACE3/ezhttp"
```

## HTTP

```go
client := fetch.Client{
    Base:    "https://api.example.com",
    Timeout: 10 * time.Second,
    Retry:   3,
    Proxy:   "socks5://localhost:1080",
    Headers: func() fetch.Headers {
        return fetch.Headers{
            "Authorization": "Bearer " + getToken(),
            "User-Agent":    "Mozilla/5.0 Chrome/120",
        }
    },
}

// JSON
var user User
err := client.Get(ctx, "/users/1").JSON(&user)

// generic
user, err := fetch.To[User](client.Get(ctx, "/users/1"))

// post
resp := client.Post(ctx, "/users", NewUser{Name: "alice"})

// path traversal — no structs needed
price := client.Get(ctx, "/api/market/BTC").Path("data", "quote", "USD", "price").Float()

// query params
client.Get(ctx, "/search" + fetch.Query{"q": "golang", "limit": "10"}.Encode())
```

## HTML Scraping

```go
doc, _ := client.Get(ctx, "/products").HTML()

// css selectors
title := doc.Find("h1").Text()
links := doc.FindAll("a.product").Attr("href")

// decode into structs with css tags
type Product struct {
    Name  string `css:".name"`
    Price string `css:".price"`
    SKU   string `css:".sku" attr:"data-id"`
}
var products []Product
doc.FindAll(".product-card").Decode(&products)
```

## Through Pipeline

```go
// chain transforms before parsing
var data MarketData
client.Get(ctx, "/api/encrypted").
    Through(decrypt).
    Through(fetch.DecodeGzip).
    JSON(&data)

// compose
decode := fetch.Chain(fetch.DecodeBase64, fetch.DecodeGzip)
client.Get(ctx, "/api/data").Through(decode).JSON(&result)
```

## WebSocket

```go
ws, _ := client.WS(ctx, "wss://stream.binance.com:9443/ws/btcusdt@ticker", fetch.WSConfig{
    Headers:   fetch.Headers{"Origin": "https://www.binance.com"},
    Reconnect: true,
})

ws.Send(map[string]any{"method": "SUBSCRIBE", "params": []string{"ethusdt@ticker"}})

for msg := range ws.Messages {
    price := msg.Path("c").String()
    msg.JSON(&ticker)
    msg.Through(decrypt).JSON(&result)
}

ws.Close()
```

## Polling

```go
stream := client.Poll(ctx, fetch.PollConfig{
    Path:     "/api/price/BTC",
    Interval: 2 * time.Second,
})
for resp := range stream.Values {
    fmt.Println(resp.Path("price").Float())
}

// multi-endpoint
stream := client.PollMany(ctx, fetch.PollConfig{
    Paths:    []string{"/price/BTC", "/price/ETH", "/price/SOL"},
    Interval: 1 * time.Second,
    StopWhen: func(v fetch.Value) bool { return !v.Path("open").Bool() },
})
```

## Browser Fingerprinting

```go
// TLS fingerprint + UA rotation + browser headers
client := fetch.Client{
    Browser: fetch.Chrome, // or Firefox, Safari, Edge, RandomBrowser()
}

// Force HTTP version
client := fetch.Client{
    Browser:   fetch.Chrome,
    ForceHTTP: fetch.HTTP2,  // HTTP1, HTTP2, HTTP3, Auto (default)
}

// Auto (default): HTTP/2 for HTTPS with fingerprint (like real browsers)
// HTTP1: force HTTP/1.1
// HTTP2: force HTTP/2
// HTTP3: force HTTP/3 (QUIC)
```

## Error Handling

```go
resp := client.Get(ctx, "/users/1")
if resp.Err() != nil {
    var re *fetch.ResponseError
    if errors.As(resp.Err(), &re) {
        re.Status     // 429
        re.Body       // error body bytes
        re.RetryAfter // parsed Retry-After header
    }
}

// or just let it flow
err := client.Get(ctx, "/users/1").JSON(&user) // network + http + parse errors
```

## Features

- **Zero-config** — `fetch.Client{Base: "..."}` just works
- **Pure net/http** — no HTTP framework deps
- **Retry** — exponential backoff on 429/5xx, respects Retry-After
- **Proxy** — HTTP/HTTPS/SOCKS5 for both HTTP and WebSocket
- **Dynamic headers** — `func() Headers` called per-request (rotating tokens, timestamps)
- **JSON path** — `resp.Path("data", "price").Float()` without structs (gjson)
- **HTML scraping** — CSS selectors + struct tag decode (goquery)
- **Through** — chainable body transforms (decrypt, decompress, etc)
- **WebSocket** — auto ping/pong, reconnect, proxy, same Message API as Response
- **Polling** — single/multi endpoint with StopWhen
- **Generics** — `fetch.To[T](resp)` one-liner typed extraction
- **Browser fingerprint** — TLS (utls) + UA rotation + sec-ch-ua/sec-fetch headers
- **HTTP/1.1, HTTP/2, HTTP/3** — force or auto-negotiate per client

### Deps

| Dep | Purpose |
|-----|---------|
| [goquery](https://github.com/PuerkitoBio/goquery) | HTML CSS selectors |
| [gjson](https://github.com/tidwall/gjson) | Zero-alloc JSON path traversal |
| [brotli](https://github.com/andybalholm/brotli) | Brotli decompression |
| [gorilla/websocket](https://github.com/gorilla/websocket) | WebSocket protocol |
| [utls](https://github.com/refraction-networking/utls) | TLS fingerprinting |
| [quic-go](https://github.com/quic-go/quic-go) | HTTP/3 (QUIC) |
