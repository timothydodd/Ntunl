package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/timothydodd/ntunl/internal/auth"
	"github.com/timothydodd/ntunl/internal/config"
	"github.com/timothydodd/ntunl/internal/host"
	"github.com/timothydodd/ntunl/internal/host/portal"
	"github.com/timothydodd/ntunl/internal/logx"
	"github.com/timothydodd/ntunl/internal/store"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "create-admin" {
		createAdmin(os.Args[2:])
		return
	}

	configPath := flag.String("config", "host.json", "path to host config JSON")
	logLevel := flag.String("loglevel", "info", "log level (debug|info|warn|error)")
	flag.Parse()

	log := logx.New(os.Stdout, logx.ParseLevel(*logLevel))
	logx.PrintLogo(os.Stdout)

	var cfg host.Config
	if err := config.Load(*configPath, &cfg); err != nil {
		log.Error("load config", "err", err)
		os.Exit(1)
	}
	applyDefaults(&cfg)

	st, err := store.Open(cfg.Database.Path)
	if err != nil {
		log.Error("open database", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := host.BootstrapAdmin(log, st); err != nil {
		log.Error("bootstrap admin", "err", err)
		os.Exit(1)
	}

	authn := auth.NewAuthenticator(st)
	tunnel := host.NewTunnelHost(log, cfg.TunnelHost, st, authn)
	httpSrv := host.NewHttpServer(log, tunnel, cfg.HttpHost)
	web := portal.New(portal.Options{
		Log:    log,
		Store:  st,
		Auth:   authn,
		Live:   tunnel,
		Domain: cfg.TunnelHost.ClientDomain.Domain,
		Port:   cfg.Portal.Port,
		Secure: cfg.Portal.Secure,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return tunnel.Run(ctx) })
	g.Go(func() error { return httpSrv.Run(ctx) })
	g.Go(func() error { return web.Run(ctx) })

	if err := g.Wait(); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func applyDefaults(cfg *host.Config) {
	if cfg.Database.Path == "" {
		cfg.Database.Path = "ntunl.db"
	}
	if cfg.Portal.Port == 0 {
		cfg.Portal.Port = 8002
	}
}

// createAdmin handles `host create-admin -username x -password y`.
func createAdmin(args []string) {
	fs := flag.NewFlagSet("create-admin", flag.ExitOnError)
	configPath := fs.String("config", "host.json", "path to host config JSON")
	username := fs.String("username", "", "admin username")
	password := fs.String("password", "", "admin password")
	_ = fs.Parse(args)

	if *username == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "usage: host create-admin -username <u> -password <p>")
		os.Exit(2)
	}

	var cfg host.Config
	if err := config.Load(*configPath, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err)
		os.Exit(1)
	}
	applyDefaults(&cfg)

	st, err := store.Open(cfg.Database.Path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open database:", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := host.CreateAdmin(st, *username, *password); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("created admin:", *username)
}
