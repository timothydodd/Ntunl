# NTunl — Auth + Admin/User Portal Plan

> **Superseded note:** the reserved-subdomain model below was later dropped in
> favor of **dynamic assignment** (client-requested name if free, else random) —
> the `subdomains` table, admin Subdomains page, and reservation requirement were
> removed. Auth, the portal, users/tokens, and tunnel handshake are all as built.


Add multi-user authentication and a web portal to NTunl:

- **Client auth:** `client login` authenticates interactively against the host,
  stores a token locally, and the tunnel presents that token on connect. No more
  anonymous subdomain grabs.
- **Host portal:** a Tailwind web portal where **admins** create/manage users and
  **users** see their tunnels, reserved subdomains, tokens, and traffic.
- **Reserved subdomains:** each user owns stable subdomain(s); their public URL no
  longer changes between sessions.

## Decisions (locked in)

| Area            | Choice                                                            |
|-----------------|------------------------------------------------------------------|
| Storage         | SQLite via `modernc.org/sqlite` (pure Go, static binary, no CGO) |
| Portal UI       | Server-rendered `html/template` styled with Tailwind             |
| Subdomains      | Reserved per user (stable URLs)                                   |
| Client auth     | Interactive `client login` → token stored on disk → sent on WS   |
| Router          | stdlib `net/http` ServeMux (Go 1.22 method+path patterns)        |
| Password hashing| `golang.org/x/crypto/bcrypt`                                      |
| Portal sessions | Server-side session table + httpOnly cookie                      |

### Tailwind without a Node pipeline
Use the **standalone Tailwind CLI** (single binary, no Node) to compile
`internal/host/portal/assets/input.css` → `assets/app.css`, scanning the
templates. Commit the built `app.css` and `go:embed` it so the binary is
self-contained. A `make tailwind` / documented command rebuilds it. For instant
iteration the layout template can fall back to the Tailwind Play CDN behind a
build tag / config flag. (Same approach will later restyle the client inspector.)

## New ports (host now runs three listeners)

| Port (default) | Purpose                                | Auth                         |
|----------------|----------------------------------------|------------------------------|
| 8001           | Tunnel WebSocket (clients)             | API token on handshake       |
| 9200           | Public HTTP proxy (outside traffic)   | none (public)                |
| 8002 (new)     | Admin/user portal + JSON auth API     | session cookie / token       |

Keep the portal on its own port and never expose it through `:9200`.

## Data model (SQLite)

```sql
users(
  id INTEGER PK, username TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'user',  -- 'admin'|'user'
  disabled INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL)

tokens(           -- API tokens used by the client tunnel
  id INTEGER PK, user_id INTEGER NOT NULL REFERENCES users(id),
  name TEXT, token_hash TEXT UNIQUE NOT NULL,  -- sha256 of the token; plaintext shown once
  created_at TEXT NOT NULL, last_used_at TEXT, revoked INTEGER NOT NULL DEFAULT 0)

subdomains(       -- reservations
  id INTEGER PK, user_id INTEGER NOT NULL REFERENCES users(id),
  name TEXT UNIQUE NOT NULL, created_at TEXT NOT NULL)

sessions(         -- portal browser sessions
  id TEXT PK,       -- random opaque id stored in cookie
  user_id INTEGER NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL, expires_at TEXT NOT NULL)

tunnel_events(    -- connection history for the dashboard
  id INTEGER PK, user_id INTEGER NOT NULL, subdomain TEXT NOT NULL,
  remote_addr TEXT, connected_at TEXT NOT NULL, disconnected_at TEXT)
```

Live state (who is connected right now, in-flight request counts, a recent-request
ring buffer per user) stays **in memory** in `TunnelHost`; the dashboard joins the
in-memory view with these tables. Token/password values are never stored in
plaintext (bcrypt for passwords, sha256 for tokens).

## New / changed package layout

```
internal/
  store/                 # NEW — SQLite access
    store.go             # open db, run migrations (embedded .sql)
    users.go tokens.go subdomains.go sessions.go events.go
    migrations/0001_init.sql
  auth/                  # NEW — auth primitives
    password.go          # bcrypt hash/verify
    token.go             # generate/hash/verify API tokens
    session.go           # cookie session issue/validate
    middleware.go        # RequireAuth, RequireAdmin (net/http)
  host/
    portal/              # NEW — the web portal
      portal.go          # http.Server wiring on :8002
      handlers_auth.go   # GET/POST /login, /logout, POST /api/auth/login
      handlers_user.go   # dashboard, tokens, change password
      handlers_admin.go  # user CRUD, subdomain assignment, active tunnels
      view.go            # template helpers / view models
      templates/*.html   # Tailwind templates (layout, login, dashboard, admin)
      assets/app.css     # built Tailwind (embedded)
    tunnel.go            # CHANGED — auth handshake + reserved-subdomain assignment
    config.go            # CHANGED — add Portal{Port}, Database{Path}, Admin bootstrap
  client/
    credentials.go       # NEW — load/save token at os.UserConfigDir()/ntunl
    login.go             # NEW — interactive login flow (x/term hidden input)
    tunnel.go            # CHANGED — send Authorization: Bearer <token> on Dial
cmd/
  host/main.go           # CHANGED — open store, bootstrap admin, start portal
  client/main.go         # CHANGED — subcommands: login | logout | run (default)
```

## Host: portal & API

Session-protected, server-rendered pages (Tailwind):

- `GET /login`, `POST /login` — form login → sets session cookie → redirect `/`.
- `GET /logout`.
- `GET /` **dashboard (user):** their reserved subdomain(s) + full public URL,
  live connected/offline + remote IP + connected-since, request count and a recent
  requests table (method, path, status, time), and a list of their API tokens.
- `GET /tokens`, `POST /tokens` (create, shows plaintext once), `POST /tokens/{id}/revoke`.
- `POST /account/password` — change own password.
- **Admin only** (`RequireAdmin`):
  - `GET /admin/users` — list; `POST /admin/users` create (username, role, temp
    password); `POST /admin/users/{id}/disable|enable|reset-password`.
  - `GET /admin/subdomains`, `POST /admin/subdomains` — reserve `name` → user;
    `POST /admin/subdomains/{id}/release`.
  - `GET /admin/tunnels` — all currently-connected tunnels across users.

CLI/programmatic auth:
- `POST /api/auth/login` `{username,password}` → `{token}` (a new API token minted
  for the client). Used by `client login`. Returns 401 on bad creds / disabled.

Middleware: `RequireAuth` (valid session or bearer token), `RequireAdmin` (role).

## Host: tunnel auth + reserved subdomains  (`internal/host/tunnel.go`)

1. On WS upgrade, read `Authorization: Bearer <token>` from the handshake request
   **before** `websocket.Accept` succeeds; look up the token → user (reject 401 /
   close if missing, revoked, or user disabled). Stamp `last_used_at`.
2. Replace `assignName()`: the client may request a desired subdomain (from its
   config); validate the **authenticated user owns** that reservation and it isn't
   already live. If the user owns exactly one, default to it. Reject if they own
   none ("no subdomain reserved — ask an admin") or the requested one isn't theirs.
3. Record a `tunnel_events` row on connect/disconnect; keep the live entry in the
   in-memory map (now also carrying `userID` for the dashboard).

The public proxy (`httpserver.go`) is largely unchanged — it still routes by
subdomain — but the subdomain set is now driven by reservations, not config.

## Client: interactive login + stored token

New multi-command CLI (`cmd/client/main.go`):

- `client login [-portal https://host:8002]` — prompt for username, then password
  (hidden via `golang.org/x/term`), `POST /api/auth/login`, store the returned
  token via `credentials.go` at `os.UserConfigDir()/ntunl/credentials.json`
  (perms 0600), keyed by host. Print "logged in as <user>".
- `client logout [-portal ...]` — delete the stored token (optionally call a
  revoke endpoint).
- `client` / `client run [-config ...]` (default) — load config, load the stored
  token for the tunnel host, open the tunnel sending `Authorization: Bearer`.
  If no token found, print "run `client login` first" and exit.

`credentials.json` shape: `{ "hosts": { "host.example.com": { "token": "..." } } }`.
The tunnel host is derived from `ntunlAddress`; the portal host from `-portal`
(defaults to the same host on the portal port). `client.json` gains an optional
`desiredSubdomain` and `portalAddress`.

## Host bootstrap (first admin)

On startup, if `users` is empty: create an admin from `NTUNL_ADMIN_USER` /
`NTUNL_ADMIN_PASSWORD` env vars, or generate a random password and **print it once**
to the log. Also a `host create-admin -username x` subcommand for manual creation.

## New dependencies

- `modernc.org/sqlite` (+ `database/sql`) — pure-Go SQLite driver.
- `golang.org/x/crypto/bcrypt` — password hashing.
- `golang.org/x/term` — hidden password prompt in the client.
- Tailwind standalone CLI — **build-time only**, not a Go dependency.

Everything else stays stdlib. (Router = `net/http`, sessions/tokens hand-rolled.)

## Config additions (`configs/host.json`)

```jsonc
{
  "portal":   { "port": 8002 },
  "database": { "path": "ntunl.db" },
  "tunnelHost": { /* ... existing; clientDomain.subDomains becomes optional */ },
  "httpHost":   { /* unchanged */ }
}
```
`clientDomain.domain` is still used to render public URLs (`https://<sub>.<domain>`).
The static `subDomains` list is superseded by DB reservations (kept as an optional
seed for first-run convenience).

## Phased execution

1. **Store + migrations** — `internal/store`, embedded `0001_init.sql`, open/migrate
   on host start. Unit tests against a temp DB.
2. **Auth primitives** — `internal/auth`: bcrypt, token gen/hash, session
   issue/validate, `RequireAuth`/`RequireAdmin` middleware. Tests.
3. **Portal skeleton** — `:8002` server, `/login`/`/logout`, session cookie, a
   minimal Tailwind layout + login page. Bootstrap-admin on first run.
4. **User dashboard + token management** — reserved subdomain + live status +
   recent requests + token create/revoke + change password.
5. **Admin pages** — user CRUD, subdomain reservation, active-tunnels view.
6. **Tunnel auth + reserved assignment** — token handshake in `tunnel.go`, replace
   `assignName`, record `tunnel_events`, thread `userID` into live state.
7. **Client CLI** — `login`/`logout`/`run`, credential storage, bearer on Dial.
8. **Tailwind build** — standalone CLI pipeline, embed `app.css`, document rebuild.
9. **Bootstrap + Docker + docs** — sqlite volume in Docker, env bootstrap, README
   ("create admin → log in → reserve subdomain → user logs in via client").
10. **(Later)** restyle the client inspector with the same Tailwind setup.

## Security checklist

- Passwords: bcrypt (cost ~12). API tokens: 32 random bytes, stored sha256, shown
  once. Sessions: opaque random id, httpOnly + SameSite=Lax (+ Secure under TLS),
  server-side expiry.
- Constant-time token/secret comparison; generic "invalid credentials" errors.
- Reject disabled users at both portal login and tunnel handshake.
- Basic login rate-limiting / lockout (nice-to-have, note if deferred).
- `credentials.json` written 0600; never log tokens/passwords.
- Portal bound to its own port; consider requiring TLS for it in production.

## Open questions / things to confirm during build

- Multiple reserved subdomains per user, or exactly one? (Plan supports many;
  default to one for simplicity.)
- Should `client login` auto-pick the portal URL from `ntunlAddress` (same host,
  portal port) to avoid a second flag? (Plan: yes, with `-portal` override.)
- Token lifetime — non-expiring + manual revoke (plan) vs. expiring tokens.
```
