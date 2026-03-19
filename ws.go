package ezhttp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// WSConfig configures a WebSocket connection.
type WSConfig struct {
	// Headers sent during the handshake.
	Headers Headers

	// Reconnect automatically on disconnect. Default: false.
	Reconnect bool

	// ReconnectWait between reconnect attempts. Default: 5s.
	ReconnectWait time.Duration

	// PingInterval for keepalive pings. Default: 30s. Set to -1 to disable.
	PingInterval time.Duration

	// BufferSize for the Messages channel. Default: 256.
	BufferSize int
}

func (cfg *WSConfig) defaults() {
	if cfg.ReconnectWait == 0 {
		cfg.ReconnectWait = 5 * time.Second
	}
	if cfg.PingInterval == 0 {
		cfg.PingInterval = 30 * time.Second
	}
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 256
	}
}

// Conn is a WebSocket connection with auto-ping, reconnect, and a message channel.
type Conn struct {
	// Messages receives incoming WebSocket messages.
	Messages <-chan *Message

	// RespHeaders from the initial handshake response.
	RespHeaders http.Header

	conn    *websocket.Conn
	msgs    chan *Message
	done    chan struct{}
	closed  atomic.Bool
	mu      sync.Mutex
	wg      sync.WaitGroup
	client  *Client
	wsURL   string
	cfg     WSConfig
	ctx     context.Context
	cancel  context.CancelFunc
}

// Message is a single WebSocket message with convenience methods mirroring Response.
type Message struct {
	Data []byte
	err  error
}

// Text returns the message as a string.
func (m *Message) Text() string {
	if m.err != nil {
		return ""
	}
	return string(m.Data)
}

// JSON unmarshals the message into dst.
func (m *Message) JSON(dst any) error {
	if m.err != nil {
		return m.err
	}
	return json.Unmarshal(m.Data, dst)
}

// Path navigates into a JSON message without unmarshaling.
func (m *Message) Path(keys ...string) Value {
	if m.err != nil || m.Data == nil {
		return Value{}
	}
	v := valueFromBytes(m.Data)
	if len(keys) == 0 {
		return v
	}
	return v.Path(keys...)
}

// Bytes returns the raw message bytes.
func (m *Message) Bytes() []byte { return m.Data }

// Err returns any error associated with this message.
func (m *Message) Err() error { return m.err }

// Through applies a transform to the message data.
func (m *Message) Through(fn ThroughFunc) *Message {
	if m.err != nil {
		return m
	}
	b, err := fn(m.Data)
	if err != nil {
		return &Message{err: err}
	}
	return &Message{Data: b}
}

// --- Client.WS ---

// WS opens a WebSocket connection using the client's proxy config.
//
//	ws := client.WS(ctx, "wss://stream.example.com/ws", fetch.WSConfig{
//	    Headers: fetch.Headers{"Origin": "https://example.com"},
//	})
//	for msg := range ws.Messages {
//	    price := msg.Path("data", "price").Float()
//	}
func (c *Client) WS(ctx context.Context, wsURL string, cfg WSConfig) (*Conn, error) {
	cfg.defaults()

	wsCtx, wsCancel := context.WithCancel(ctx)
	msgs := make(chan *Message, cfg.BufferSize)

	conn := &Conn{
		Messages: msgs,
		msgs:     msgs,
		done:     make(chan struct{}),
		client:   c,
		wsURL:    wsURL,
		cfg:      cfg,
		ctx:      wsCtx,
		cancel:   wsCancel,
	}

	if err := conn.dial(); err != nil {
		c.logWS("error", wsURL)
		wsCancel()
		return nil, err
	}
	c.logWS("open", wsURL)

	conn.wg.Add(1)
	go conn.readLoop()

	if cfg.PingInterval > 0 {
		conn.wg.Add(1)
		go conn.pingLoop()
	}

	if cfg.Reconnect {
		conn.wg.Add(1)
		go conn.reconnectLoop()
	}

	return conn, nil
}

// Send marshals v as JSON and sends it.
func (c *Conn) Send(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return ErrConnClosed
	}
	return c.conn.WriteJSON(v)
}

// SendText sends a text message.
func (c *Conn) SendText(s string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return ErrConnClosed
	}
	return c.conn.WriteMessage(websocket.TextMessage, []byte(s))
}

// SendBytes sends a binary message.
func (c *Conn) SendBytes(b []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return ErrConnClosed
	}
	return c.conn.WriteMessage(websocket.BinaryMessage, b)
}

// Close permanently shuts down the connection and drains goroutines.
func (c *Conn) Close() error {
	if c.closed.Swap(true) {
		return nil // already closed
	}
	c.client.logWS("closed", c.wsURL)
	c.cancel()
	close(c.done)

	c.mu.Lock()
	var err error
	if c.conn != nil {
		err = c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()

	c.wg.Wait()
	return err
}

// IsClosed returns whether the connection has been permanently closed.
func (c *Conn) IsClosed() bool {
	return c.closed.Load()
}

// ErrConnClosed is returned when sending on a closed connection.
var ErrConnClosed = &WSError{Msg: "connection closed"}

// WSError is a WebSocket error.
type WSError struct {
	Msg string
}

func (e *WSError) Error() string { return "ezhttp: ws: " + e.Msg }

// --- internal ---

func (c *Conn) dial() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		TLSClientConfig:  &tls.Config{},
	}

	// Inherit proxy from client.
	if c.client.Proxy != "" {
		proxyURL, err := url.Parse(c.client.Proxy)
		if err != nil {
			return err
		}
		if proxyURL.Scheme == "socks5" || proxyURL.Scheme == "socks5h" {
			dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, network, proxyURL.Host)
			}
		} else {
			dialer.Proxy = http.ProxyURL(proxyURL)
		}
	}

	h := http.Header{}
	for k, v := range c.cfg.Headers {
		h.Set(k, v)
	}
	// Apply client-level dynamic headers too.
	if c.client.Headers != nil {
		for k, v := range c.client.Headers() {
			if h.Get(k) == "" { // WSConfig headers take precedence
				h.Set(k, v)
			}
		}
	}
	if h.Get("User-Agent") == "" {
		h.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	}

	ws, resp, err := dialer.DialContext(c.ctx, c.wsURL, h)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = ws
	if resp != nil {
		c.RespHeaders = resp.Header
	}
	c.mu.Unlock()

	return nil
}

func (c *Conn) readLoop() {
	defer c.wg.Done()
	for {
		c.mu.Lock()
		ws := c.conn
		c.mu.Unlock()

		if ws == nil {
			// No connection — wait for reconnect or shutdown.
			select {
			case <-c.done:
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}

		_, data, err := ws.ReadMessage()
		if err != nil {
			if c.closed.Load() {
				return
			}
			// Signal disconnect for reconnect loop.
			c.mu.Lock()
			c.conn = nil
			c.mu.Unlock()

			if !c.cfg.Reconnect {
				// No reconnect — close messages channel and exit.
				close(c.msgs)
				return
			}
			continue
		}

		msg := &Message{Data: data}
		select {
		case c.msgs <- msg:
		case <-c.done:
			return
		default:
			// Drop message if buffer is full.
		}
	}
}

func (c *Conn) pingLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(c.cfg.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			ws := c.conn
			c.mu.Unlock()
			if ws != nil {
				ws.WriteMessage(websocket.PingMessage, nil)
			}
		case <-c.done:
			return
		}
	}
}

func (c *Conn) reconnectLoop() {
	defer c.wg.Done()
	for {
		select {
		case <-c.done:
			return
		case <-time.After(c.cfg.ReconnectWait):
			if c.closed.Load() {
				return
			}
			c.mu.Lock()
			connected := c.conn != nil
			c.mu.Unlock()
			if connected {
				continue
			}
			// Attempt reconnect.
			if err := c.dial(); err != nil {
				continue // retry next interval
			}
			c.client.logWS("reconnected", c.wsURL)
		}
	}
}
