package client

import (
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Credentials is the on-disk token store, keyed by host (hostname without port).
type Credentials struct {
	Hosts map[string]HostCred `json:"hosts"`
}

// HostCred is a stored token for one host.
type HostCred struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

// credentialsPath returns <os.UserConfigDir>/ntunl/credentials.json.
func credentialsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ntunl", "credentials.json"), nil
}

// LoadCredentials reads the credential store, returning an empty one if absent.
func LoadCredentials() (*Credentials, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Credentials{Hosts: map[string]HostCred{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.Hosts == nil {
		c.Hosts = map[string]HostCred{}
	}
	return &c, nil
}

// Save writes the credential store with 0600 perms.
func (c *Credentials) Save() error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Get returns the stored credential for a host address (any of "host",
// "host:port", or "https://host:port").
func (c *Credentials) Get(addr string) (HostCred, bool) {
	hc, ok := c.Hosts[HostKey(addr)]
	return hc, ok
}

// Set stores a credential for a host address.
func (c *Credentials) Set(addr string, hc HostCred) {
	if c.Hosts == nil {
		c.Hosts = map[string]HostCred{}
	}
	c.Hosts[HostKey(addr)] = hc
}

// Delete removes a credential for a host address.
func (c *Credentials) Delete(addr string) {
	delete(c.Hosts, HostKey(addr))
}

// HostKey normalizes an address to a bare hostname so the portal URL
// ("https://host:8002") and the tunnel address ("host:8001") map to the same key.
func HostKey(addr string) string {
	s := addr
	if i := strings.Index(s, "://"); i >= 0 {
		if u, err := url.Parse(s); err == nil && u.Hostname() != "" {
			return strings.ToLower(u.Hostname())
		}
		s = s[i+3:]
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		return strings.ToLower(h)
	}
	return strings.ToLower(s)
}
