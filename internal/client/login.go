package client

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// Login prompts for username/password, authenticates against the portal at
// portalURL, and stores the returned token keyed by each tunnel host so the
// `run` command can find it (the portal and tunnel may be different hostnames).
func Login(portalURL string, tunnelAddrs []string, insecure bool) error {
	portalURL = strings.TrimRight(portalURL, "/")

	username, err := promptLine("Username: ")
	if err != nil {
		return err
	}
	password, err := promptPassword("Password: ")
	if err != nil {
		return err
	}

	body, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
		"name":     "ntunl-client",
	})

	httpClient := &http.Client{Timeout: 15 * time.Second}
	if insecure {
		httpClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}

	resp, err := httpClient.Post(portalURL+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("connect to portal: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed (%s)", resp.Status)
	}

	var out struct {
		Token    string `json:"token"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	creds, err := LoadCredentials()
	if err != nil {
		return err
	}
	if len(tunnelAddrs) == 0 {
		// Fall back to keying by the portal host if no tunnels were provided.
		tunnelAddrs = []string{portalURL}
	}
	keys := make([]string, 0, len(tunnelAddrs))
	for _, addr := range tunnelAddrs {
		creds.Set(addr, HostCred{Token: out.Token, Username: out.Username})
		keys = append(keys, HostKey(addr))
	}
	if err := creds.Save(); err != nil {
		return err
	}

	fmt.Printf("Logged in as %s (token stored for host %s)\n", out.Username, strings.Join(keys, ", "))
	return nil
}

// Logout removes the stored credentials for the given hosts.
func Logout(hostAddrs []string) error {
	creds, err := LoadCredentials()
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(hostAddrs))
	for _, addr := range hostAddrs {
		creds.Delete(addr)
		keys = append(keys, HostKey(addr))
	}
	if err := creds.Save(); err != nil {
		return err
	}
	fmt.Printf("Logged out of host %s\n", strings.Join(keys, ", "))
	return nil
}

func promptLine(label string) (string, error) {
	fmt.Print(label)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptPassword(label string) (string, error) {
	fmt.Print(label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
