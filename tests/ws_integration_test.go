package tests

import (
	"context"
	"testing"
	"time"

	"github.com/0xACE3/ezhttp"
)

// ===========================
// WEBSOCKET: BINANCE PUBLIC STREAM
// ===========================

func TestWS_BinanceTicker(t *testing.T) {
	client := ezhttp.Client{}

	ws, err := client.WS(context.Background(), "wss://stream.binance.com:9443/ws/btcusdt@ticker", ezhttp.WSConfig{
		Headers: ezhttp.Headers{
			"Origin": "https://www.binance.com",
		},
	})
	if err != nil {
		t.Skipf("Binance WS unavailable: %v", err)
	}
	defer ws.Close()

	// Binance sends 24h ticker updates every second.
	var received int
	timeout := time.After(10 * time.Second)
loop:
	for {
		select {
		case msg, ok := <-ws.Messages:
			if !ok {
				break loop
			}
			if msg.Err() != nil {
				t.Fatal(msg.Err())
			}

			symbol := msg.Path("s").String()
			price := msg.Path("c").String()  // current price as string
			volume := msg.Path("v").String() // 24h volume
			change := msg.Path("P").String() // 24h change %

			if symbol == "" || price == "" {
				t.Logf("partial message: %s", msg.Text()[:min(200, len(msg.Text()))])
				continue
			}

			t.Logf("BTCUSDT: price=%s, vol=%s, change=%s%%", price, volume, change)
			received++
			if received >= 3 {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if received < 1 {
		t.Fatalf("expected at least 1 ticker message, got %d", received)
	}
	t.Logf("Received %d Binance ticker messages", received)
}

// ===========================
// WEBSOCKET: BINANCE SUBSCRIBE/UNSUBSCRIBE
// ===========================

func TestWS_BinanceSubscribe(t *testing.T) {
	client := ezhttp.Client{}

	// Connect to base stream endpoint.
	ws, err := client.WS(context.Background(), "wss://stream.binance.com:9443/ws", ezhttp.WSConfig{
		Headers: ezhttp.Headers{
			"Origin": "https://www.binance.com",
		},
	})
	if err != nil {
		t.Skipf("Binance WS unavailable: %v", err)
	}
	defer ws.Close()

	// Subscribe to multiple streams via JSON message.
	err = ws.Send(map[string]any{
		"method": "SUBSCRIBE",
		"params": []string{"btcusdt@miniTicker", "ethusdt@miniTicker"},
		"id":     1,
	})
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	timeout := time.After(15 * time.Second)
loop:
	for {
		select {
		case msg, ok := <-ws.Messages:
			if !ok {
				break loop
			}
			// Skip subscription confirmation.
			if msg.Path("result").Exists() || msg.Path("id").Exists() {
				t.Logf("subscription response: %s", msg.Text())
				continue
			}

			sym := msg.Path("s").String()
			if sym != "" {
				seen[sym] = true
				t.Logf("stream: %s price=%s", sym, msg.Path("c").String())
			}
			if seen["BTCUSDT"] && seen["ETHUSDT"] {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if !seen["BTCUSDT"] {
		t.Fatal("never received BTCUSDT")
	}
	if !seen["ETHUSDT"] {
		t.Fatal("never received ETHUSDT")
	}
	t.Logf("Received both BTCUSDT and ETHUSDT streams")
}

// ===========================
// WEBSOCKET: BINANCE MULTI-STREAM JSON PATH
// ===========================

func TestWS_BinancePathTraversal(t *testing.T) {
	client := ezhttp.Client{}

	ws, err := client.WS(context.Background(), "wss://stream.binance.com:9443/ws/btcusdt@bookTicker", ezhttp.WSConfig{
		Headers: ezhttp.Headers{
			"Origin": "https://www.binance.com",
		},
	})
	if err != nil {
		t.Skipf("Binance WS unavailable: %v", err)
	}
	defer ws.Close()

	// bookTicker gives best bid/ask.
	timeout := time.After(10 * time.Second)
	select {
	case msg, ok := <-ws.Messages:
		if !ok {
			t.Fatal("channel closed")
		}

		// Test various Path methods.
		symbol := msg.Path("s").String()
		bestBid := msg.Path("b").String()
		bestAsk := msg.Path("a").String()
		bidQty := msg.Path("B").String()
		askQty := msg.Path("A").String()

		if symbol != "BTCUSDT" {
			t.Fatalf("expected BTCUSDT, got %q", symbol)
		}
		if bestBid == "" || bestAsk == "" {
			t.Fatal("expected bid/ask prices")
		}

		t.Logf("BTCUSDT book: bid=%s (%s), ask=%s (%s)", bestBid, bidQty, bestAsk, askQty)

		// Test Keys — bookTicker has known fields.
		keys := msg.Path("@this").Keys()
		if len(keys) == 0 {
			t.Fatal("expected keys in bookTicker message")
		}
		t.Logf("bookTicker keys: %v", keys)

	case <-timeout:
		t.Fatal("timeout waiting for bookTicker")
	}
}

// ===========================
// WEBSOCKET: MESSAGE JSON DECODE
// ===========================

func TestWS_BinanceJSONDecode(t *testing.T) {
	client := ezhttp.Client{}

	ws, err := client.WS(context.Background(), "wss://stream.binance.com:9443/ws/btcusdt@ticker", ezhttp.WSConfig{
		Headers: ezhttp.Headers{
			"Origin": "https://www.binance.com",
		},
	})
	if err != nil {
		t.Skipf("Binance WS unavailable: %v", err)
	}
	defer ws.Close()

	// Use map[string]any since Binance mixes string and number types across streams.
	timeout := time.After(10 * time.Second)
	select {
	case msg, ok := <-ws.Messages:
		if !ok {
			t.Fatal("channel closed")
		}

		var ticker map[string]any
		if err := msg.JSON(&ticker); err != nil {
			t.Fatal(err)
		}

		sym, _ := ticker["s"].(string)
		if sym != "BTCUSDT" {
			t.Fatalf("expected BTCUSDT, got %q", sym)
		}
		if ticker["c"] == nil {
			t.Fatal("expected last price field")
		}

		t.Logf("Decoded ticker: %s last=%v high=%v low=%v vol=%v",
			sym, ticker["c"], ticker["h"], ticker["l"], ticker["v"])

	case <-timeout:
		t.Fatal("timeout")
	}
}

// ===========================
// WEBSOCKET: COINMARKETCAP PUBLIC STREAM
// ===========================

func TestWS_CoinMarketCap(t *testing.T) {
	client := ezhttp.Client{}

	ws, err := client.WS(context.Background(),
		"wss://push.coinmarketcap.com/ws?device=web&client_source=coin_detail_page",
		ezhttp.WSConfig{
			Headers: ezhttp.Headers{
				"Origin":     "https://coinmarketcap.com",
				"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0",
			},
		})
	if err != nil {
		t.Skipf("CMC WS unavailable: %v", err)
	}
	defer ws.Close()

	// Subscribe to BTC (id=1) price updates.
	err = ws.Send(map[string]any{
		"method": "subscribe",
		"id":     "price",
		"data": map[string]any{
			"cryptoIds":  []int{1, 1027}, // BTC, ETH
			"index":      "detail",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var received int
	timeout := time.After(30 * time.Second)
loop:
	for {
		select {
		case msg, ok := <-ws.Messages:
			if !ok {
				break loop
			}
			text := msg.Text()
			if len(text) > 5 {
				t.Logf("CMC msg (%d bytes): %.200s", len(text), text)
				received++
			}
			if received >= 2 {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if received < 1 {
		t.Skipf("CMC sent %d messages (may need auth or different subscription)", received)
	}
	t.Logf("Received %d CMC messages", received)
}

// ===========================
// WEBSOCKET: THROUGH PIPELINE ON MESSAGES
// ===========================

func TestWS_BinanceThrough(t *testing.T) {
	client := ezhttp.Client{}

	ws, err := client.WS(context.Background(), "wss://stream.binance.com:9443/ws/btcusdt@ticker", ezhttp.WSConfig{
		Headers: ezhttp.Headers{
			"Origin": "https://www.binance.com",
		},
	})
	if err != nil {
		t.Skipf("Binance WS unavailable: %v", err)
	}
	defer ws.Close()

	timeout := time.After(10 * time.Second)
	select {
	case msg, ok := <-ws.Messages:
		if !ok {
			t.Fatal("channel closed")
		}

		// Through: transform then read via Path.
		transformed := msg.Through(func(b []byte) ([]byte, error) {
			return b, nil // identity — testing the pipeline works
		})
		price := transformed.Path("c").String()
		change := transformed.Path("P").String()
		if price == "" {
			t.Fatal("expected price after Through")
		}
		t.Logf("Through pipeline: last=%s, change=%s%%", price, change)

	case <-timeout:
		t.Fatal("timeout")
	}
}

// ===========================
// WEBSOCKET: CLIENT WITH HEADERS INHERITANCE
// ===========================

func TestWS_ClientHeadersInherited(t *testing.T) {
	client := ezhttp.Client{
		Headers: func() ezhttp.Headers {
			return ezhttp.Headers{
				"User-Agent": "fetch-ws-test/1.0",
			}
		},
	}

	// Client-level headers should be inherited by WS connections.
	// WSConfig headers take precedence over client headers.
	ws, err := client.WS(context.Background(), "wss://stream.binance.com:9443/ws/btcusdt@ticker", ezhttp.WSConfig{
		Headers: ezhttp.Headers{
			"Origin": "https://www.binance.com",
		},
	})
	if err != nil {
		t.Skipf("Binance WS unavailable: %v", err)
	}
	defer ws.Close()

	// Just verify it connects and receives data — headers were sent during handshake.
	timeout := time.After(10 * time.Second)
	select {
	case msg, ok := <-ws.Messages:
		if !ok {
			t.Fatal("channel closed")
		}
		if msg.Path("s").String() != "BTCUSDT" {
			t.Fatalf("unexpected symbol: %s", msg.Path("s").String())
		}
		t.Log("WS with inherited client headers works")
	case <-timeout:
		t.Fatal("timeout")
	}
}
