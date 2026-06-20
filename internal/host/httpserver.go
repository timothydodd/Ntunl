package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/timothydodd/ntunl/internal/protocol"
)

const requestTimeout = 20 * time.Second

// HttpServer is the public-facing HTTP listener. It maps each request's
// subdomain to a connected tunnel client and forwards the request.
type HttpServer struct {
	log      *slog.Logger
	tunnel   *TunnelHost
	settings HttpHostSettings

	blacklist     map[string]struct{}
	blacklistWild []string
	ipHeaderName  string
	defaultCode   int
}

// NewHttpServer builds the public HTTP forwarder.
func NewHttpServer(log *slog.Logger, tunnel *TunnelHost, settings HttpHostSettings) *HttpServer {
	s := &HttpServer{
		log:          log,
		tunnel:       tunnel,
		settings:     settings,
		blacklist:    make(map[string]struct{}),
		ipHeaderName: settings.Headers.IpHeaderName,
		defaultCode:  settings.DefaultResponseCode,
	}
	if s.ipHeaderName == "" {
		s.ipHeaderName = "X-Forwarded-For"
	}
	if s.defaultCode == 0 {
		s.defaultCode = 404
	}
	for _, item := range settings.Headers.BlackList {
		if strings.Contains(item, "*") {
			s.blacklistWild = append(s.blacklistWild, strings.TrimRight(item, "*"))
		} else {
			s.blacklist[strings.ToLower(item)] = struct{}{}
		}
	}
	return s
}

// Run starts the HTTP listener and blocks until ctx is cancelled.
func (s *HttpServer) Run(ctx context.Context) error {
	addr := listenAddr(s.settings.HostName, s.settings.Port)
	srv := &http.Server{Addr: addr, Handler: http.HandlerFunc(s.handle)}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	s.log.Info("HttpServer is running", "addr", addr)
	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *HttpServer) handle(w http.ResponseWriter, r *http.Request) {
	clientName := strings.Split(r.Host, ".")[0]

	var client *clientInfo
	if clientName == "localhost" || clientName == "192" {
		client = s.tunnel.GetAnyClient()
	} else {
		client = s.tunnel.GetClient(clientName)
	}
	if client == nil {
		s.writeNotFound(w)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Error("read request body", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	headers, clientIP := s.collectHeaders(r)

	reqData := &protocol.HttpRequestData{
		Method:  r.Method,
		Path:    r.URL.RequestURI(),
		Content: body,
		Headers: headers,
		ContentHeaders: map[string]string{
			"Content-Type":   valueOr(r.Header.Get("Content-Type"), "application/octet-stream"),
			"Content-Length": strconv.Itoa(len(body)),
		},
	}

	s.log.Info(fmt.Sprintf("%s => %s: %s", clientIP, r.Method, reqData.Path))

	resp, err := s.tunnel.SendHttpRequest(r.Context(), reqData, client, requestTimeout)
	if err != nil {
		s.log.Error("forward request failed", "err", err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}

	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	if enc, ok := resp.ContentHeaders["Content-Encoding"]; ok {
		w.Header().Set("Content-Encoding", enc)
	}
	if ct, ok := resp.ContentHeaders["Content-Type"]; ok {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	status := resp.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	if _, err := w.Write(resp.Content); err != nil {
		s.log.Error("write response", "err", err, "path", reqData.Path)
	}
}

// collectHeaders copies request headers, dropping blacklisted ones and pulling
// the client IP out of the configured IP header.
func (s *HttpServer) collectHeaders(r *http.Request) (map[string]string, string) {
	clientIP := "unknown"
	out := make(map[string]string)
	for key, vals := range r.Header {
		if len(vals) == 0 {
			continue
		}
		if _, blocked := s.blacklist[strings.ToLower(key)]; blocked {
			continue
		}
		if strings.EqualFold(key, s.ipHeaderName) {
			clientIP = vals[0]
			continue
		}
		if s.matchesWild(key) {
			continue
		}
		out[key] = strings.Join(vals, ",")
	}
	// net/http strips Host from r.Header; preserve it so the client can override.
	if r.Host != "" {
		out["Host"] = r.Host
	}
	return out, clientIP
}

func (s *HttpServer) matchesWild(key string) bool {
	for _, prefix := range s.blacklistWild {
		if strings.HasPrefix(strings.ToLower(key), strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// writeNotFound serves a generic HTML page (with the configured status code) when
// no client is connected for the requested subdomain.
func (s *HttpServer) writeNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(s.defaultCode)
	_, _ = io.WriteString(w, notFoundPage)
}

const notFoundPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Tunnel not available</title>
<style>
  html,body{height:100%;margin:0}
  body{display:flex;align-items:center;justify-content:center;
       font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;
       background:#0f172a;color:#e2e8f0}
  .card{text-align:center;padding:2.5rem 3rem}
  .code{font-size:4rem;font-weight:700;letter-spacing:-2px;color:#38bdf8;margin:0}
  h1{font-size:1.25rem;font-weight:600;margin:.5rem 0 .25rem}
  p{color:#94a3b8;margin:.25rem 0}
</style>
</head>
<body>
  <div class="card">
    <p class="code">404</p>
    <h1>Tunnel not available</h1>
    <p>No client is currently connected for this address.</p>
  </div>
</body>
</html>`
