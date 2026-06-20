package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/timothydodd/ntunl/internal/protocol"
)

const (
	retryInterval = 5 * time.Second
	maxRetries    = 10
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
	for ctx.Err() == nil {
		conn, err := c.connect(ctx)
		if err != nil {
			c.log.Error("giving up connecting", "address", c.state.settings.NtunlAddress, "err", err)
			return
		}
		c.serve(ctx, conn)
		if ctx.Err() != nil {
			return
		}
		c.log.Info("Client: disconnected from server, will retry")
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

// serve reads commands until the connection closes.
func (c *tunnelClient) serve(ctx context.Context, conn *websocket.Conn) {
	defer conn.Close(websocket.StatusNormalClosure, "")
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		cmd, err := protocol.Unmarshal(data)
		if err != nil {
			c.log.Error("bad command from server", "err", err)
			continue
		}
		c.dispatch(ctx, conn, cmd)
	}
}

func (c *tunnelClient) dispatch(ctx context.Context, conn *websocket.Conn, cmd *protocol.Command) {
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
	case protocol.CmdHttpRequest:
		resp, err := c.handler.handle(ctx, cmd, c.state)
		if err != nil {
			c.log.Error("error handling message", "err", err)
			return
		}
		raw, err := resp.Marshal()
		if err != nil {
			c.log.Error("marshal response", "err", err)
			return
		}
		if err := conn.Write(ctx, websocket.MessageBinary, raw); err != nil {
			c.log.Error("write response", "err", err)
		}
	}
}
