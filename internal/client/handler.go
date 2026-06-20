package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/timothydodd/ntunl/internal/compress"
	"github.com/timothydodd/ntunl/internal/protocol"
)

// RequestLog is one request/response pair retained for the inspector.
type RequestLog struct {
	Request  *protocol.HttpRequestData
	Response *protocol.HttpResponseData
}

// tunnelState holds per-tunnel settings plus the public URL pushed by the host.
type tunnelState struct {
	settings       TunnelSetting
	rewriteRegex   *regexp.Regexp
	rewriteEnabled bool

	mu        sync.RWMutex
	ntunlInfo *protocol.NtunlInfo
}

func newTunnelState(settings TunnelSetting) (*tunnelState, error) {
	st := &tunnelState{settings: settings}
	if settings.RewriteUrlEnabled && strings.TrimSpace(settings.RewriteUrlPattern) != "" {
		re, err := regexp.Compile(settings.RewriteUrlPattern)
		if err != nil {
			return nil, fmt.Errorf("compile rewrite pattern: %w", err)
		}
		st.rewriteRegex = re
		st.rewriteEnabled = true
	}
	return st, nil
}

func (s *tunnelState) setInfo(info *protocol.NtunlInfo) {
	s.mu.Lock()
	s.ntunlInfo = info
	s.mu.Unlock()
}

func (s *tunnelState) url() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ntunlInfo == nil {
		return ""
	}
	return s.ntunlInfo.Url
}

// messageHandler replays forwarded requests against the local target. One handler
// is shared by all tunnels (matching the original singleton).
type messageHandler struct {
	log    *slog.Logger
	client *http.Client

	mu   sync.Mutex
	logs []RequestLog
}

func newMessageHandler(log *slog.Logger) *messageHandler {
	return &messageHandler{
		log:    log,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// snapshot returns a copy of the retained request logs.
func (h *messageHandler) snapshot() []RequestLog {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]RequestLog, len(h.logs))
	copy(out, h.logs)
	return out
}

// handle processes one HttpRequest command and produces the HttpResponse command
// to send back to the host.
func (h *messageHandler) handle(ctx context.Context, cmd *protocol.Command, state *tunnelState) (*protocol.Command, error) {
	if strings.TrimSpace(cmd.Data) == "" {
		return nil, fmt.Errorf("no data in request")
	}

	var reqData protocol.HttpRequestData
	if err := json.Unmarshal([]byte(cmd.Data), &reqData); err != nil {
		return nil, fmt.Errorf("deserialize request: %w", err)
	}

	target := compress.CombineUrlPath(state.settings.Address, reqData.Path)
	started := time.Now()

	var bodyReader io.Reader
	if len(reqData.Content) > 0 {
		bodyReader = bytes.NewReader(reqData.Content)
	}

	req, err := http.NewRequestWithContext(ctx, methodOrGet(reqData.Method), target, bodyReader)
	if err != nil {
		return nil, err
	}

	for k, v := range reqData.Headers {
		if k == "Host" && strings.TrimSpace(state.settings.HostHeader) != "" {
			req.Host = state.settings.HostHeader
			continue
		}
		if custom, ok := state.settings.CustomHeader[k]; ok {
			req.Header.Set(k, custom)
			continue
		}
		if k == "Content-Length" || k == "Content-Type" {
			continue
		}
		req.Header.Add(k, v)
	}
	if ct, ok := reqData.ContentHeaders["Content-Type"]; ok && len(reqData.Content) > 0 {
		req.Header.Set("Content-Type", ct)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s -> %s: %w", reqData.Method, reqData.Path, target, err)
	}
	defer resp.Body.Close()

	respData, err := h.buildResponse(resp, state)
	if err != nil {
		return nil, err
	}

	h.log.Info("proxied",
		"method", reqData.Method,
		"path", reqData.Path,
		"status", respData.StatusCode,
		"ms", time.Since(started).Milliseconds())

	respBytes, err := json.Marshal(respData)
	if err != nil {
		return nil, err
	}

	h.mu.Lock()
	h.logs = append(h.logs, RequestLog{Request: &reqData, Response: respData})
	h.mu.Unlock()

	return &protocol.Command{
		CommandType:    protocol.CmdHttpResponse,
		ConversationId: cmd.ConversationId,
		Data:           string(respBytes),
	}, nil
}

func (h *messageHandler) buildResponse(resp *http.Response, state *tunnelState) (*protocol.HttpResponseData, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string)
	contentHeaders := make(map[string]string)
	for k, v := range resp.Header {
		joined := strings.Join(v, ",")
		if isContentHeader(k) {
			contentHeaders[k] = joined
		} else {
			headers[k] = joined
		}
	}
	contentHeaders["Content-Length"] = strconv.Itoa(len(body))

	out := &protocol.HttpResponseData{
		StatusCode:     resp.StatusCode,
		Content:        body,
		Headers:        headers,
		ContentHeaders: contentHeaders,
	}

	if state.rewriteEnabled {
		if ct, ok := contentHeaders["Content-Type"]; ok && strings.Contains(ct, "text/html") {
			if url := state.url(); url != "" {
				if err := h.rewriteBody(out, state, url); err != nil {
					h.log.Error("url rewrite failed", "err", err)
				}
			}
		}
	}

	return out, nil
}

// rewriteBody decompresses the response body, applies the URL rewrite, and
// recompresses with the original encoding.
func (h *messageHandler) rewriteBody(data *protocol.HttpResponseData, state *tunnelState, url string) error {
	enc := compress.EncodeNone
	switch data.ContentHeaders["Content-Encoding"] {
	case "br":
		dec, err := compress.BrotliDecompress(data.Content)
		if err != nil {
			return err
		}
		data.Content = dec
		enc = compress.EncodeBrotli
	case "gzip":
		dec, err := compress.GzipDecompress(data.Content)
		if err != nil {
			return err
		}
		data.Content = dec
		enc = compress.EncodeGzip
	}

	rewritten := state.rewriteRegex.ReplaceAll(data.Content, []byte(url))
	recompressed, err := compress.Compress(rewritten, enc)
	if err != nil {
		return err
	}
	data.Content = recompressed
	data.ContentHeaders["Content-Length"] = strconv.Itoa(len(recompressed))
	return nil
}

func isContentHeader(k string) bool {
	switch strings.ToLower(k) {
	case "content-type", "content-length", "content-encoding", "content-language",
		"content-range", "content-disposition", "content-location", "expires", "last-modified", "allow":
		return true
	}
	return false
}

func methodOrGet(m string) string {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodDelete:
		return m
	default:
		return http.MethodGet
	}
}
