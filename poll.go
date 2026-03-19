package ezhttp

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// PollConfig configures a polling stream.
type PollConfig struct {
	// Path for single-endpoint polling (used by Poll).
	Path string

	// Paths for multi-endpoint polling (used by PollMany).
	Paths []string

	// Interval between poll cycles. Default: 10s.
	Interval time.Duration

	// StopWhen terminates the stream when it returns true.
	// Called with the JSON Value of each response. Optional.
	StopWhen func(Value) bool
}

// Stream delivers poll results over a channel.
type Stream struct {
	Values <-chan *Response
	stop   context.CancelFunc
	wg     sync.WaitGroup
}

// Stop terminates the polling stream and waits for goroutines to finish.
func (s *Stream) Stop() {
	s.stop()
	s.wg.Wait()
}

// Poll starts polling a single endpoint at the configured interval.
func (c *Client) Poll(ctx context.Context, cfg PollConfig) *Stream {
	cfg.Paths = []string{cfg.Path}
	return c.PollMany(ctx, cfg)
}

// PollMany starts polling multiple endpoints at the configured interval.
// All endpoints are fetched concurrently each cycle.
func (c *Client) PollMany(ctx context.Context, cfg PollConfig) *Stream {
	if cfg.Interval == 0 {
		cfg.Interval = 10 * time.Second
	}

	ctx, cancel := context.WithCancel(ctx)
	ch := make(chan *Response, len(cfg.Paths)*2)
	s := &Stream{Values: ch, stop: cancel}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer close(ch)

		// Fetch immediately, then on tick.
		c.pollOnce(ctx, cfg, ch)

		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if c.pollOnce(ctx, cfg, ch) {
					return // StopWhen triggered
				}
			}
		}
	}()

	return s
}

func (c *Client) pollOnce(ctx context.Context, cfg PollConfig, ch chan<- *Response) bool {
	var (
		wg      sync.WaitGroup
		stopped atomic.Bool
	)
	for _, path := range cfg.Paths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			resp := c.Get(ctx, p)
			select {
			case ch <- resp:
			case <-ctx.Done():
				return
			}
			if cfg.StopWhen != nil && resp.err == nil {
				v := valueFromBytes(resp.Body)
				if cfg.StopWhen(v) {
					stopped.Store(true)
				}
			}
		}(path)
	}
	wg.Wait()
	return stopped.Load()
}
