package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/timothydodd/ntunl/internal/protocol"
)

const (
	retryInterval = 5 * time.Second
	maxRetries    = 10

	// pingInterval keeps the tunnel alive across idle proxies (e.g. Cloudflare)
	// and detects a silently-dead connection so we can reconnect.
	pingInterval = 30 * time.Second
	pingTimeout  = 10 * time.Second
	writeTimeout = 30 * time.Second

	// maxConcurrentRequests bounds how many forwarded requests we process at once
	// so the read loop never stalls and the local service isn't flooded.
	maxConcurrentRequests = 100
)

// tunnelClient maintains one WebSocket connection to the host and serves
// forwarded requests through the shared message handler.
type tunnelClient struct {
	log     *slog.Logger
	handler *messageHandler
	state   *tunnelState
	token   string
}

func newTunnelClient(log *slog.Logger, handler *messageHandler, settings TunnelSetting, token string) (*tunnelClient, error) {
	state, err := newTunnelState(settings)
	if err != nil {
		return nil, err
	}
	return &tunnelClient{log: log, handler: handler, state: state, token: token}, nil
}

// run connects (with retry) and serves until ctx is cancelled. It reconnects if
// the connection drops while ctx is still live.
func (c *tunnelClient) run(ctx context.Context) {
	addr := c.state.settings.NtunlAddress
	for ctx.Err() == nil {
		conn, err := c.connect(ctx)
		if err != nil {
			c.log.Error("giving up connecting", "address", addr, "err", err)
			return
		}
		c.serve(ctx, conn)
		if ctx.Err() != nil {
			c.log.Info("Client: shutting down", "address", addr)
			return
		}
		c.log.Info("Client: disconnected, will retry", "address", addr, "in", retryInterval.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryInterval):
		}
	}
}

func (c *tunnelClient) connect(ctx context.Context) (*websocket.Conn, error) {
	scheme := "ws"
	if c.state.settings.SslEnabled {
		scheme = "wss"
	}
	url := fmt.Sprintf("%s://%s/", scheme, c.state.settings.NtunlAddress)

	opts := &websocket.DialOptions{HTTPHeader: http.Header{}}
	opts.HTTPHeader.Set("Authorization", "Bearer "+c.token)
	if sub := c.state.settings.DesiredSubdomain; sub != "" {
		opts.HTTPHeader.Set("Ntunl-Subdomain", sub)
	}
	if c.state.settings.SslEnabled && c.state.settings.AllowInvalidCertificates {
		opts.HTTPClient = &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		}
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetries && ctx.Err() == nil; attempt++ {
		c.log.Info("Client: connecting", "url", url, "attempt", attempt)
		conn, _, err := websocket.Dial(ctx, url, opts)
		if err == nil {
			conn.SetReadLimit(-1)
			c.log.Info("Client: connected to server", "url", url)
			return conn, nil
		}
		lastErr = err
		c.log.Warn(fmt.Sprintf("Connection attempt %d failed", attempt), "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryInterval):
		}
	}
	return nil, lastErr
}

// serve runs the read loop. Each forwarded request is handled in its own
// goroutine so the loop keeps reading; a keepalive pinger detects dead peers.
func (c *tunnelClient) serve(ctx context.Context, conn *websocket.Conn) {
	// connCtx cancels this connection's work (handlers, pinger) on shutdown or
	// when the read loop exits — closing the conn unblocks a stuck Read.
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var writeMu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrentRequests)

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.keepalive(connCtx, conn)
	}()

readLoop:
	for {
		_, data, err := conn.Read(connCtx)
		if err != nil {
			if connCtx.Err() != nil {
				c.log.Debug("read loop stopped (context cancelled)")
			} else {
				c.log.Warn("Client: connection closed", "err", err)
			}
			break
		}

		cmd, err := protocol.Unmarshal(data)
		if err != nil {
			c.log.Error("bad command from server", "err", err)
			continue
		}

		switch cmd.CommandType {
		case protocol.CmdHttpRequest:
			select {
			case sem <- struct{}{}:
			case <-connCtx.Done():
				break readLoop
			}
			wg.Add(1)
			go func(cmd *protocol.Command) {
				defer wg.Done()
				defer func() { <-sem }()
				c.handleRequest(connCtx, conn, &writeMu, cmd)
			}(cmd)
		default:
			c.dispatchControl(cmd)
		}
	}

	cancel()
	conn.Close(websocket.StatusNormalClosure, "")
	wg.Wait()
}

// handleRequest processes one forwarded HTTP request and writes the response.
func (c *tunnelClient) handleRequest(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, cmd *protocol.Command) {
	resp, err := c.handler.handle(ctx, cmd, c.state)
	if err != nil {
		if ctx.Err() == nil {
			c.log.Error("error handling request", "err", err)
		}
		return
	}
	raw, err := resp.Marshal()
	if err != nil {
		c.log.Error("marshal response", "err", err)
		return
	}
	if err := c.writeMsg(ctx, conn, writeMu, raw); err != nil {
		if ctx.Err() == nil {
			c.log.Error("write response", "err", err)
		}
	}
}

// writeMsg serializes writes (coder/websocket allows only one writer) and bounds
// each write with a timeout so a stuck write can't wedge a handler forever.
func (c *tunnelClient) writeMsg(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, raw []byte) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return conn.Write(wctx, websocket.MessageBinary, raw)
}

// dispatchControl handles non-request messages (echo, url info).
func (c *tunnelClient) dispatchControl(cmd *protocol.Command) {
	switch cmd.CommandType {
	case protocol.CmdEcho:
		c.log.Info(cmd.Data)
	case protocol.CmdNtunlInfo:
		var info protocol.NtunlInfo
		if err := json.Unmarshal([]byte(cmd.Data), &info); err != nil {
			c.log.Error("bad ntunl info", "err", err)
			return
		}
		c.state.setInfo(&info)
		c.log.Info("Your Url: " + info.Url)
	}
}

// keepalive pings the peer periodically; a failed ping closes the connection so
// the read loop unblocks and run() reconnects.
func (c *tunnelClient) keepalive(ctx context.Context, conn *websocket.Conn) {
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, pingTimeout)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				if ctx.Err() == nil {
					c.log.Warn("keepalive ping failed; reconnecting", "err", err)
					conn.Close(websocket.StatusGoingAway, "ping timeout")
				}
				return
			}
		}
	}
}
