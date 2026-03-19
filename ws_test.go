package fetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// echoServer echoes back any message it receives.
func echoServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer ws.Close()
		for {
			mt, msg, err := ws.ReadMessage()
			if err != nil {
				return
			}
			ws.WriteMessage(mt, msg)
		}
	}))
}

// jsonServer sends JSON messages on a ticker, then closes.
func jsonServer(t *testing.T, count int, interval time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		for i := range count {
			msg := map[string]any{"seq": i, "data": map[string]any{"price": 42000.5 + float64(i)}}
			ws.WriteJSON(msg)
			if i < count-1 {
				time.Sleep(interval)
			}
		}
	}))
}

func TestWS_SendReceiveText(t *testing.T) {
	srv := echoServer(t)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := Client{}

	ws, err := client.WS(context.Background(), wsURL, WSConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	// Send text, receive echo.
	if err := ws.SendText("hello fetch"); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-ws.Messages:
		if msg.Text() != "hello fetch" {
			t.Fatalf("got %q, want 'hello fetch'", msg.Text())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for echo")
	}
}

func TestWS_SendReceiveJSON(t *testing.T) {
	srv := echoServer(t)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := Client{}

	ws, err := client.WS(context.Background(), wsURL, WSConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	// Send JSON, receive echo, parse with Path.
	payload := map[string]any{"action": "subscribe", "symbols": []string{"BTC", "ETH"}}
	if err := ws.Send(payload); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-ws.Messages:
		action := msg.Path("action").String()
		if action != "subscribe" {
			t.Fatalf("got action %q, want subscribe", action)
		}
		// Decode symbols array.
		var syms []string
		msg.Path("symbols").Each(func(i int, v Value) {
			syms = append(syms, v.String())
		})
		if len(syms) != 2 || syms[0] != "BTC" || syms[1] != "ETH" {
			t.Fatalf("got symbols %v", syms)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestWS_MessageThrough(t *testing.T) {
	srv := echoServer(t)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := Client{}

	ws, err := client.WS(context.Background(), wsURL, WSConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	ws.SendText(`{"wrapped":"inner_data"}`)

	select {
	case msg := <-ws.Messages:
		// Through: extract the "wrapped" value as new data.
		transformed := msg.Through(func(b []byte) ([]byte, error) {
			var m map[string]json.RawMessage
			json.Unmarshal(b, &m)
			return m["wrapped"], nil
		})
		if transformed.Text() != `"inner_data"` {
			t.Fatalf("got %q", transformed.Text())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestWS_StreamJSON(t *testing.T) {
	srv := jsonServer(t, 5, 50*time.Millisecond)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := Client{}

	ws, err := client.WS(context.Background(), wsURL, WSConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	var received int
	timeout := time.After(3 * time.Second)
	for {
		select {
		case msg, ok := <-ws.Messages:
			if !ok {
				goto done
			}
			seq := msg.Path("seq").Int()
			price := msg.Path("data", "price").Float()
			t.Logf("msg %d: price=%.1f", seq, price)
			received++
			if received >= 5 {
				goto done
			}
		case <-timeout:
			goto done
		}
	}
done:
	if received != 5 {
		t.Fatalf("expected 5 messages, got %d", received)
	}
}

func TestWS_MessageJSON(t *testing.T) {
	srv := echoServer(t)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := Client{}

	ws, err := client.WS(context.Background(), wsURL, WSConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	type Payload struct {
		Name string `json:"name"`
		Val  int    `json:"val"`
	}

	ws.Send(Payload{Name: "test", Val: 42})

	select {
	case msg := <-ws.Messages:
		var p Payload
		if err := msg.JSON(&p); err != nil {
			t.Fatal(err)
		}
		if p.Name != "test" || p.Val != 42 {
			t.Fatalf("got %+v", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestWS_Close(t *testing.T) {
	srv := echoServer(t)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := Client{}

	ws, err := client.WS(context.Background(), wsURL, WSConfig{})
	if err != nil {
		t.Fatal(err)
	}

	ws.Close()

	if !ws.IsClosed() {
		t.Fatal("expected closed")
	}

	// Send should fail after close.
	if err := ws.SendText("fail"); err != ErrConnClosed {
		t.Fatalf("expected ErrConnClosed, got %v", err)
	}
}

func TestWS_Headers(t *testing.T) {
	var gotUA string
	var gotOrigin string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotOrigin = r.Header.Get("Origin")
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ws.Close()
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := Client{}

	ws, err := client.WS(context.Background(), wsURL, WSConfig{
		Headers: Headers{
			"User-Agent": "custom-agent/1.0",
			"Origin":     "https://myapp.com",
		},
	})
	if err != nil {
		// Connection will close immediately, that's fine.
		// Headers were already sent during handshake.
		_ = err
	}
	if ws != nil {
		ws.Close()
	}

	if gotUA != "custom-agent/1.0" {
		t.Fatalf("expected custom UA, got %q", gotUA)
	}
	if gotOrigin != "https://myapp.com" {
		t.Fatalf("expected custom origin, got %q", gotOrigin)
	}
}

func TestWS_Reconnect(t *testing.T) {
	connections := make(chan struct{}, 10)
	msgCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connections <- struct{}{}
		// Send one message then close — forces reconnect.
		ws.WriteJSON(map[string]int{"n": msgCount})
		msgCount++
		time.Sleep(100 * time.Millisecond)
		ws.Close()
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := Client{}

	ws, err := client.WS(context.Background(), wsURL, WSConfig{
		Reconnect:     true,
		ReconnectWait: 500 * time.Millisecond,
		PingInterval:  -1, // disable ping for this test
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	// Should receive messages across reconnects.
	var received int
	timeout := time.After(5 * time.Second)
loop:
	for {
		select {
		case msg, ok := <-ws.Messages:
			if !ok {
				break loop
			}
			n := msg.Path("n").Int()
			t.Logf("received msg n=%d", n)
			received++
			if received >= 3 {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if received < 2 {
		t.Fatalf("expected at least 2 messages across reconnects, got %d", received)
	}
	t.Logf("received %d messages across reconnects", received)
}
