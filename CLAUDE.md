# CLAUDE.md

Guidance for working in this repository.

## What this is

NTunl is a lightweight ngrok-style HTTP tunnel: expose a local/private service to
the public internet through a public relay. It was **migrated from .NET 9 to Go**;
the original .NET source is preserved under `_legacy/` for reference and is no
longer built. See `plans/go-migration.md` for the migration plan and rationale.

## Architecture

Two binaries plus shared internal packages. Each WebSocket message is one
JSON-encoded `protocol.Command` (no hand-rolled binary framing — that was dropped
from the .NET version).

- **`host`** (`cmd/host`, `internal/host`) — the public-facing relay (the "server"
  role, analogous to ngrok's cloud). Runs three listeners:
  - a WebSocket server (default `:8001`) that tunnel clients dial into. Clients
    **authenticate with a bearer API token** on the handshake; the host maps the
    token to a user and **assigns a subdomain** (`assignSubdomain` in tunnel.go),
    then pushes back the public URL. Two modes, set by `clientDomain.subDomains`:
    - **Pool** (list non-empty): assign a free name from that fixed list (honoring
      a requested one if it's a free pool member), reject when exhausted. Route a
      handful of subdomains to the host — no wildcard DNS needed.
    - **Fully dynamic** (empty list): requested name if free, else a random
      `word+number` (needs wildcard DNS to be routable).
    Either way it's first-come-first-served per live connection; nothing is
    pre-reserved per user.
  - a public HTTP server (default `:9200`) that receives outside traffic, maps the
    request's subdomain (`apple.domain.com` → client `apple`) to a connected
    client, forwards the request over the socket, relays the response, and counts
    requests per user.
  - a **portal** (default `:8002`, `internal/host/portal`) — Tailwind web UI + auth
    API. Users see their live tunnels/URLs, recent requests, and manage API
    tokens; admins create/disable users and view all active tunnels. `client login`
    hits `POST /api/auth/login` here.
  - Request/response matching: `SendHttpRequest` registers a
    `map[conversationId]chan *HttpResponseData`; the per-connection read pump
    routes each reply to the waiting channel. Writes are serialized per client.
- **`client`** (`cmd/client`, `internal/client`) — a CLI with subcommands
  `login` | `logout` | `run` (default). `login` prompts for credentials, gets a
  token, stores it at `os.UserConfigDir()/ntunl/credentials.json` (keyed by host).
  `run` dials out (with reconnect), sending the token + desired subdomain, then
  replays each forwarded request against the configured `address`. Supports
  multiple tunnels, optional URL rewriting, and an optional inspector page.
- **Persistence** (`internal/store`) — SQLite (`modernc.org/sqlite`, pure Go):
  `users`, `tokens` (sha256-hashed), `sessions`, `tunnel_events` (connection
  history). Subdomains are NOT persisted — they're assigned dynamically per live
  connection and tracked only in `TunnelHost`'s in-memory map. Schema in
  `migrations/`, applied on open.
- **Auth** (`internal/auth`) — bcrypt passwords, random API tokens (sha256
  stored), portal session cookies, `RequireAuth`/`RequireAdmin` middleware,
  bearer-token → user resolution (used by the tunnel handshake).
- **Shared** (`internal/`): `protocol` (Command + DTOs), `compress` (gzip/brotli +
  URL-path join), `certs` (self-signed PEM cert generate-or-load), `logx` (colored
  slog handler + ASCII logo), `config` (JSON loader).

### First-run / bootstrap
- On first start with an empty DB, the host creates an admin from
  `NTUNL_ADMIN_USER`/`NTUNL_ADMIN_PASSWORD`, else the default **`admin`/`admin`**
  (`host.DefaultAdminUser`/`DefaultAdminPassword`), logged with a warning to
  change it. Users/admins change their own password via the dashboard
  (`/account/password`). `host create-admin -username x -password y` also works.

### Routing notes / known gaps
- Host catch-all: requests whose subdomain is `localhost` or `192` route to *any*
  connected client (handy for local testing). A bare public IP does **not** match,
  so real deployments need wildcard DNS (`*.domain.com`) + subdomain routing.
- Tunnels require a valid token; anonymous connects are rejected. Subdomains are
  dynamic (requested-if-free, else random) — no reservation needed. The portal
  currently uses the **Tailwind Play CDN** (see Phase 8 in
  `plans/auth-and-portal.md` for the standalone-CLI static build to do later).

## Build & run

`go` is **not installed in this WSL environment** — the user builds/runs on
Windows. Do not attempt `go build`/`go run` here; write the code and let the user
run it. The repo lives on a shared mount (`/mnt/f/...` ≡ `F:\github\Ntunl`).

```
go build ./...                                   # compile-check all packages
go run ./cmd/host   -config configs/host.json    # public relay
go run ./cmd/client -config configs/client.json  # tunnel client
go test ./...                                     # unit tests (protocol, compress)
```

Local end-to-end smoke test (one machine): run a service on `:8080`, start the
host, start the client with `configs/client.local.json`, then
`curl http://localhost:9200/` — it routes through host → client → `:8080`.
Inspector at `http://localhost:6900`.

### Config
- `configs/host.json`, `configs/client.json`, `configs/client.local.json`.
- JSON keys are camelCase, but `encoding/json` matches case-insensitively, so the
  PascalCase examples in `README.md` also bind.
- Path override: `-config <path>` flag or `NTUNL_CONFIG` env var.

## Conventions
- Standard Go layout: `cmd/` entrypoints, `internal/` packages. Module path
  `github.com/timothydodd/ntunl`.
- Keep third-party deps minimal — currently `coder/websocket`,
  `andybalholm/brotli`, `google/uuid`, `golang.org/x/sync`; everything else stdlib.
- `andybalholm/brotli` is an unpruned (`go 1.13`) module, so its indirect dep
  `xyproto/randomstring` must stay listed in `go.mod`; run `go mod tidy` after
  changing deps.
- Don't modify anything under `_legacy/` — it's reference-only.
- Do not add co-authored trailers to git commit messages.
```
