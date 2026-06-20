package client

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Client wires the shared request handler, every configured tunnel, and the
// optional inspector together.
type Client struct {
	log      *slog.Logger
	cfg      Config
	handler  *messageHandler
	tunnels  []*tunnelClient
}

// New builds a Client from config, resolving each tunnel's stored auth token.
func New(log *slog.Logger, cfg Config) (*Client, error) {
	creds, err := LoadCredentials()
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}

	handler := newMessageHandler(log)
	c := &Client{log: log, cfg: cfg, handler: handler}
	for _, t := range cfg.Tunnels {
		hc, ok := creds.Get(t.NtunlAddress)
		if !ok || hc.Token == "" {
			return nil, fmt.Errorf("no stored token for host %q — run `client login` first", HostKey(t.NtunlAddress))
		}
		tc, err := newTunnelClient(log, handler, t, hc.Token)
		if err != nil {
			return nil, err
		}
		c.tunnels = append(c.tunnels, tc)
	}
	return c, nil
}

// Run starts every tunnel and the inspector, blocking until ctx is cancelled.
func (c *Client) Run(ctx context.Context) error {
	var wg sync.WaitGroup

	for _, tc := range c.tunnels {
		wg.Add(1)
		go func(tc *tunnelClient) {
			defer wg.Done()
			tc.run(ctx)
		}(tc)
	}

	if c.cfg.Inspector.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runInspector(ctx, c.log, c.handler, c.cfg.Inspector.Port); err != nil {
				c.log.Error("inspector stopped", "err", err)
			}
		}()
	} else {
		c.log.Info("Inspector is disabled")
	}

	wg.Wait()
	return nil
}
