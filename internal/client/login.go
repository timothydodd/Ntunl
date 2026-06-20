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
// portalURL, and stores the returned token in the credential file.
func Login(portalURL string, insecure bool) error {
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
	creds.Set(portalURL, HostCred{Token: out.Token, Username: out.Username})
	if err := creds.Save(); err != nil {
		return err
	}

	fmt.Printf("Logged in as %s (host %s)\n", out.Username, HostKey(portalURL))
	return nil
}

// Logout removes the stored credential for a host.
func Logout(hostAddr string) error {
	creds, err := LoadCredentials()
	if err != nil {
		return err
	}
	creds.Delete(hostAddr)
	if err := creds.Save(); err != nil {
		return err
	}
	fmt.Printf("Logged out of host %s\n", HostKey(hostAddr))
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
