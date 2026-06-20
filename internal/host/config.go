package host

// Config mirrors the host config JSON shape.
type Config struct {
	TunnelHost TunnelHostSettings `json:"tunnelHost"`
	HttpHost   HttpHostSettings   `json:"httpHost"`
	Portal     PortalSettings     `json:"portal"`
	Database   DatabaseSettings   `json:"database"`
}

type PortalSettings struct {
	Port   int  `json:"port"`
	Secure bool `json:"secure"` // set Secure flag on session cookie (behind TLS)
}

type DatabaseSettings struct {
	Path string `json:"path"`
}

type TunnelHostSettings struct {
	HostName     string               `json:"hostName"`
	Port         int                  `json:"port"`
	ClientDomain ClientDomainSettings `json:"clientDomain"`
	Ssl          *SslSettings         `json:"ssl"`
}

type ClientDomainSettings struct {
	Domain     string   `json:"domain"`
	SubDomains []string `json:"subDomains"`
}

type SslSettings struct {
	Enabled                   bool `json:"enabled"`
	AcceptInvalidCertificates bool `json:"acceptInvalidCertificates"`
}

type HttpHostSettings struct {
	HostName            string              `json:"hostName"`
	Port                int                 `json:"port"`
	Headers             HttpHeaderSettings  `json:"headers"`
	DefaultResponseCode int                 `json:"defaultResponseCode"`
}

type HttpHeaderSettings struct {
	BlackList    []string `json:"blackList"`
	IpHeaderName string   `json:"ipHeaderName"`
}
