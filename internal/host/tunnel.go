package host

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/timothydodd/ntunl/internal/auth"
	"github.com/timothydodd/ntunl/internal/certs"
	"github.com/timothydodd/ntunl/internal/host/portal"
	"github.com/timothydodd/ntunl/internal/protocol"
	"github.com/timothydodd/ntunl/internal/store"
)

// fallbackWords seed randomly-generated subdomains when a client doesn't request
// a specific name.
var fallbackWords = []string{
	"apple", "banana", "cherry", "date", "elderberry", "fig", "grape", "honeydew",
	"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta",
}

// SubdomainHeader is the handshake header by which a client requests one of its
// reserved subdomains.
const SubdomainHeader = "Ntunl-Subdomain"

// clientInfo tracks a connected, authenticated tunnel client.
type clientInfo struct {
	id          string
	userID      int64
	name        string // assigned subdomain
	remoteAddr  string
	connectedAt time.Time
	eventID     int64
	requests    int64 // atomic
	conn        *websocket.Conn
	writeMu     sync.Mutex
}

// TunnelHost is the authenticated WebSocket server that accepts tunnel clients,
// binds each to one of its reserved subdomains, and forwards HTTP requests.
type TunnelHost struct {
	log      *slog.Logger
	settings TunnelHostSettings
	store    *store.Store
	auth     *auth.Authenticator

	mu      sync.RWMutex
	clients map[string]*clientInfo // keyed by lowercased subdomain
	pending map[string]chan *protocol.HttpResponseData

	reqMu  sync.Mutex
	recent map[int64][]portal.LiveRequest // per-user ring buffer

	rngMu sync.Mutex
	rng   *rand.Rand
}

const recentRequestCap = 100

// NewTunnelHost builds a TunnelHost.
func NewTunnelHost(log *slog.Logger, settings TunnelHostSettings, st *store.Store, a *auth.Authenticator) *TunnelHost {
	return &TunnelHost{
		log:      log,
		settings: settings,
		store:    st,
		auth:     a,
		clients:  make(map[string]*clientInfo),
		pending:  make(map[string]chan *protocol.HttpResponseData),
		recent:   make(map[int64][]portal.LiveRequest),
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Run starts the WebSocket listener and blocks until ctx is cancelled.
func (h *TunnelHost) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.handleConn)

	addr := listenAddr(h.settings.HostName, h.settings.Port)
	srv := &http.Server{Addr: addr, Handler: mux}

	if h.settings.Ssl != nil && h.settings.Ssl.Enabled {
		cert, err := certs.GetOrCreate("cert.pem", "key.pem")
		if err != nil {
			return fmt.Errorf("load cert: %w", err)
		}
		srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	h.log.Info("TunnelHost is running", "addr", addr)

	var err error
	if srv.TLSConfig != nil {
		err = srv.ListenAndServeTLS("", "")
	} else {
		err = srv.ListenAndServe()
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (h *TunnelHost) handleConn(w http.ResponseWriter, r *http.Request) {
	// Authenticate the bearer token BEFORE upgrading.
	token := auth.BearerToken(r)
	if token == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	user, err := h.auth.ResolveToken(token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// Assign a subdomain: the client's requested name if free, else a random one.
	name, err := h.assignSubdomain(r.Header.Get(SubdomainHeader))
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		h.log.Error("websocket accept failed", "err", err)
		h.releaseName(name)
		return
	}
	conn.SetReadLimit(-1)

	client := &clientInfo{
		id:          uuid.NewString(),
		userID:      user.ID,
		name:        name,
		remoteAddr:  r.RemoteAddr,
		connectedAt: time.Now(),
		conn:        conn,
	}
	h.mu.Lock()
	h.clients[lower(name)] = client
	h.mu.Unlock()

	if eid, err := h.store.OpenTunnelEvent(user.ID, name, r.RemoteAddr); err == nil {
		client.eventID = eid
	}

	ctx := r.Context()
	if domain := h.settings.ClientDomain.Domain; domain != "" {
		info := protocol.NtunlInfo{Url: fmt.Sprintf("https://%s.%s", name, domain)}
		data, _ := json.Marshal(info)
		_ = h.send(ctx, conn, &client.writeMu, protocol.CmdNtunlInfo, uuid.NewString(), string(data))
	}

	h.log.Info("Client connected", "user", user.Username, "subdomain", name)
	h.readPump(ctx, client)
}

// assignSubdomain picks a free subdomain and marks it live via a placeholder so
// two connections can't claim it concurrently. Behavior depends on whether a
// fixed pool is configured (clientDomain.subDomains):
//
//   - Pool mode: only names in the pool are routable (each is DNS-routed to the
//     host). A requested name is honored if it's a free pool member; otherwise the
//     first free pool member is assigned. Rejected when the pool is exhausted.
//   - Fully dynamic (empty pool): a requested name is used if free; otherwise a
//     random word+number is generated (requires wildcard DNS to be routable).
func (h *TunnelHost) assignSubdomain(requested string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	pool := h.settings.ClientDomain.SubDomains
	if len(pool) > 0 {
		// Prefer the requested name if it is a free member of the pool.
		if requested != "" {
			for _, name := range pool {
				if lower(name) == lower(requested) {
					if _, live := h.clients[lower(name)]; !live {
						h.clients[lower(name)] = nil // placeholder
						return name, nil
					}
					break // in pool but taken — fall through to next free member
				}
			}
		}
		for _, name := range pool {
			if _, live := h.clients[lower(name)]; !live {
				h.clients[lower(name)] = nil // placeholder
				return name, nil
			}
		}
		return "", errors.New("no subdomains available")
	}

	// Fully dynamic: requested-if-free, else a random word+number.
	if requested != "" {
		if _, live := h.clients[lower(requested)]; live {
			return "", errors.New("requested subdomain already in use")
		}
		h.clients[lower(requested)] = nil // placeholder
		return requested, nil
	}

	for attempts := 0; attempts < 1000; attempts++ {
		word := fmt.Sprintf("%s%d", fallbackWords[h.randInt(len(fallbackWords))], h.randInt(999)+1)
		if _, live := h.clients[lower(word)]; !live {
			h.clients[lower(word)] = nil // placeholder
			return word, nil
		}
	}
	return "", errors.New("could not allocate a free subdomain")
}

// randInt returns a serialized random int in [0,n); rand.Rand is not concurrency
// safe so callers go through this lock.
func (h *TunnelHost) randInt(n int) int {
	h.rngMu.Lock()
	defer h.rngMu.Unlock()
	return h.rng.Intn(n)
}

func (h *TunnelHost) releaseName(name string) {
	h.mu.Lock()
	if c := h.clients[lower(name)]; c == nil {
		delete(h.clients, lower(name))
	}
	h.mu.Unlock()
}

// readPump reads messages from a client and routes HttpResponse messages to the
// waiting SendHttpRequest caller by conversation id.
func (h *TunnelHost) readPump(ctx context.Context, client *clientInfo) {
	defer h.dropClient(client)
	for {
		_, data, err := client.conn.Read(ctx)
		if err != nil {
			return
		}
		cmd, err := protocol.Unmarshal(data)
		if err != nil {
			h.log.Error("bad command from client", "err", err)
			continue
		}
		if cmd.CommandType != protocol.CmdHttpResponse {
			continue
		}

		var resp protocol.HttpResponseData
		if cmd.Data != "" {
			if err := json.Unmarshal([]byte(cmd.Data), &resp); err != nil {
				h.log.Error("bad http response", "err", err)
				continue
			}
		}

		h.mu.Lock()
		ch, ok := h.pending[cmd.ConversationId]
		if ok {
			delete(h.pending, cmd.ConversationId)
		}
		h.mu.Unlock()
		if ok {
			ch <- &resp
		}
	}
}

func (h *TunnelHost) dropClient(client *clientInfo) {
	h.mu.Lock()
	if h.clients[lower(client.name)] == client {
		delete(h.clients, lower(client.name))
	}
	h.mu.Unlock()
	if client.eventID != 0 {
		_ = h.store.CloseTunnelEvent(client.eventID)
	}
	_ = client.conn.Close(websocket.StatusNormalClosure, "")
	h.log.Info("Client disconnected", "subdomain", client.name)
}

// SendHttpRequest forwards req to client and waits for the matching response or
// the timeout.
func (h *TunnelHost) SendHttpRequest(ctx context.Context, req *protocol.HttpRequestData, client *clientInfo, timeout time.Duration) (*protocol.HttpResponseData, error) {
	conversationId := uuid.NewString()
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	ch := make(chan *protocol.HttpResponseData, 1)
	h.mu.Lock()
	h.pending[conversationId] = ch
	h.mu.Unlock()

	if err := h.send(ctx, client.conn, &client.writeMu, protocol.CmdHttpRequest, conversationId, string(body)); err != nil {
		h.mu.Lock()
		delete(h.pending, conversationId)
		h.mu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		atomic.AddInt64(&client.requests, 1)
		h.recordRequest(client.userID, req.Method, req.Path, resp.StatusCode)
		return resp, nil
	case <-time.After(timeout):
		h.mu.Lock()
		delete(h.pending, conversationId)
		h.mu.Unlock()
		return nil, fmt.Errorf("timeout waiting for client response")
	case <-ctx.Done():
		h.mu.Lock()
		delete(h.pending, conversationId)
		h.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (h *TunnelHost) send(ctx context.Context, conn *websocket.Conn, mu *sync.Mutex, t protocol.CommandType, conversationId, data string) error {
	cmd := protocol.Command{CommandType: t, ConversationId: conversationId, Data: data}
	raw, err := cmd.Marshal()
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	return conn.Write(ctx, websocket.MessageBinary, raw)
}

// GetClient returns the live client for a subdomain, or nil.
func (h *TunnelHost) GetClient(name string) *clientInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.clients[lower(name)]
}

// GetAnyClient returns an arbitrary connected client, or nil.
func (h *TunnelHost) GetAnyClient() *clientInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.clients {
		if c != nil {
			return c
		}
	}
	return nil
}

func (h *TunnelHost) recordRequest(userID int64, method, path string, status int) {
	h.reqMu.Lock()
	defer h.reqMu.Unlock()
	buf := append(h.recent[userID], portal.LiveRequest{
		Time: time.Now(), Method: method, Path: path, Status: status,
	})
	if len(buf) > recentRequestCap {
		buf = buf[len(buf)-recentRequestCap:]
	}
	h.recent[userID] = buf
}

// --- portal.LiveTunnels implementation ---

func (h *TunnelHost) liveTunnel(c *clientInfo) portal.LiveTunnel {
	return portal.LiveTunnel{
		UserID:      c.userID,
		Subdomain:   c.name,
		RemoteAddr:  c.remoteAddr,
		ConnectedAt: c.connectedAt,
		Requests:    atomic.LoadInt64(&c.requests),
	}
}

// ActiveByUser returns the user's currently-connected tunnels.
func (h *TunnelHost) ActiveByUser(userID int64) []portal.LiveTunnel {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var out []portal.LiveTunnel
	for _, c := range h.clients {
		if c != nil && c.userID == userID {
			out = append(out, h.liveTunnel(c))
		}
	}
	return out
}

// AllActive returns every currently-connected tunnel.
func (h *TunnelHost) AllActive() []portal.LiveTunnel {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var out []portal.LiveTunnel
	for _, c := range h.clients {
		if c != nil {
			out = append(out, h.liveTunnel(c))
		}
	}
	return out
}

// RecentRequests returns the most recent proxied requests for a user, newest
// first, capped at limit.
func (h *TunnelHost) RecentRequests(userID int64, limit int) []portal.LiveRequest {
	h.reqMu.Lock()
	defer h.reqMu.Unlock()
	buf := h.recent[userID]
	out := make([]portal.LiveRequest, 0, len(buf))
	for i := len(buf) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, buf[i])
	}
	return out
}

func listenAddr(hostName string, port int) string {
	if hostName == "" || hostName == "*" {
		return fmt.Sprintf(":%d", port)
	}
	return net.JoinHostPort(hostName, fmt.Sprintf("%d", port))
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
