package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/timothydodd/ntunl/internal/client"
	"github.com/timothydodd/ntunl/internal/config"
	"github.com/timothydodd/ntunl/internal/logx"
)

func main() {
	cmd := "run"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}

	switch cmd {
	case "login":
		runLogin(args)
	case "logout":
		runLogout(args)
	case "run":
		runTunnel(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q (use: login | logout | run)\n", cmd)
		os.Exit(2)
	}
}

func runTunnel(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", "client.json", "path to client config JSON")
	logLevel := fs.String("loglevel", "debug", "log level (debug|info|warn|error)")
	_ = fs.Parse(args)

	log := logx.New(os.Stdout, logx.ParseLevel(*logLevel))
	logx.PrintLogo(os.Stdout)

	var cfg client.Config
	if err := config.Load(*configPath, &cfg); err != nil {
		log.Error("load config", "err", err)
		os.Exit(1)
	}

	c, err := client.New(log, cfg)
	if err != nil {
		log.Error("build client", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := c.Run(ctx); err != nil {
		log.Error("client stopped", "err", err)
		os.Exit(1)
	}
}

func runLogin(args []string) {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	portal := fs.String("portal", "", "portal URL (e.g. http://host:8002); derived from config if omitted")
	configPath := fs.String("config", "client.json", "client config used to derive the portal URL")
	insecure := fs.Bool("insecure", false, "skip TLS verification (self-signed portal)")
	_ = fs.Parse(args)

	cfg, _ := loadClientConfig(*configPath)

	url := *portal
	if url == "" {
		url = cfg.PortalAddress
	}
	if url == "" {
		url = derivePortalURL(cfg)
	}
	if url == "" {
		fmt.Fprintln(os.Stderr, "could not determine portal URL; set portalAddress in config or pass -portal https://portal.host")
		os.Exit(2)
	}

	hosts := tunnelHosts(cfg)
	if err := client.Login(url, hosts, *insecure); err != nil {
		fmt.Fprintln(os.Stderr, "login failed:", err)
		os.Exit(1)
	}
}

func runLogout(args []string) {
	fs := flag.NewFlagSet("logout", flag.ExitOnError)
	host := fs.String("host", "", "tunnel host to log out of (defaults to config's tunnel hosts)")
	configPath := fs.String("config", "client.json", "client config used to derive the host")
	_ = fs.Parse(args)

	var hosts []string
	if *host != "" {
		hosts = []string{*host}
	} else {
		cfg, _ := loadClientConfig(*configPath)
		hosts = tunnelHosts(cfg)
	}
	if len(hosts) == 0 {
		fmt.Fprintln(os.Stderr, "could not determine host; pass -host <tunnel-host>")
		os.Exit(2)
	}

	if err := client.Logout(hosts); err != nil {
		fmt.Fprintln(os.Stderr, "logout failed:", err)
		os.Exit(1)
	}
}

func loadClientConfig(configPath string) (client.Config, error) {
	var cfg client.Config
	err := config.Load(configPath, &cfg)
	return cfg, err
}

// tunnelHosts returns the NtunlAddress of every tunnel in the config.
func tunnelHosts(cfg client.Config) []string {
	hosts := make([]string, 0, len(cfg.Tunnels))
	for _, t := range cfg.Tunnels {
		if t.NtunlAddress != "" {
			hosts = append(hosts, t.NtunlAddress)
		}
	}
	return hosts
}

// derivePortalURL builds a fallback portal URL (http://<host>:8002) from the first
// tunnel's NtunlAddress — only correct when the portal is on the same host as the
// tunnel; otherwise set portalAddress in config or pass -portal.
func derivePortalURL(cfg client.Config) string {
	if len(cfg.Tunnels) == 0 {
		return ""
	}
	addr := cfg.Tunnels[0].NtunlAddress
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	if host == "" {
		return ""
	}
	return fmt.Sprintf("http://%s:8002", host)
}
