package client

// Config mirrors the client appsettings.json shape.
type Config struct {
	Tunnels   []TunnelSetting `json:"tunnels"`
	Inspector InspectorConfig `json:"inspector"`
	// PortalAddress is the base URL of the host portal for `client login`
	// (e.g. https://portal.example.com). When empty, login falls back to
	// http://<tunnelHost>:8002. Use this when the portal is a different hostname
	// than the tunnel endpoint (e.g. behind an ingress / Cloudflare).
	PortalAddress string `json:"portalAddress"`
}

type TunnelSetting struct {
	SslEnabled               bool              `json:"sslEnabled"`
	AllowInvalidCertificates bool              `json:"allowInvalidCertificates"`
	NtunlAddress             string            `json:"ntunlAddress"`
	DesiredSubdomain         string            `json:"desiredSubdomain"`
	Address                  string            `json:"address"`
	HostHeader               string            `json:"hostHeader"`
	CustomHeader             map[string]string `json:"customHeader"`
	RewriteUrlEnabled        bool              `json:"rewriteUrlEnabled"`
	RewriteUrlPattern        string            `json:"rewriteUrlPattern"`
}

type InspectorConfig struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
}
