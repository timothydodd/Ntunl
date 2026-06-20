package portal

import "time"

// LiveTunnel is a currently-connected tunnel as seen by the host's in-memory
// state. Implemented by the TunnelHost.
type LiveTunnel struct {
	UserID      int64
	Subdomain   string
	RemoteAddr  string
	ConnectedAt time.Time
	Requests    int64
}

// LiveRequest is one recently-proxied request for the dashboard.
type LiveRequest struct {
	Time   time.Time
	Method string
	Path   string
	Status int
}

// LiveTunnels exposes the host's live connection state to the portal. The
// TunnelHost satisfies this interface.
type LiveTunnels interface {
	ActiveByUser(userID int64) []LiveTunnel
	AllActive() []LiveTunnel
	RecentRequests(userID int64, limit int) []LiveRequest
}
