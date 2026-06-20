# NTunl → Go Migration Plan

Migrate the .NET 9 NTunl tunnel (host + client) to Go. **Clean break**: both
sides rewritten together; we are free to design a simpler wire protocol. Existing
.NET source moves to `_legacy/` for reference.

## 0. Decisions (locked in)

- **Strategy:** Clean break — Go host only needs to talk to a Go client. No
  byte-for-byte compat with the .NET `BinaryWriter` framing required.
- **Layout:** Single Go module at repo root, `cmd/` + `internal/`.

## 1. Target repo layout

```
/                         (go.mod: module github.com/timothydodd/ntunl)
  _legacy/                 ← entire current src/ moved here, untouched
  cmd/
    host/main.go           ← was NtunlHost
    client/main.go         ← was NtunlClient
  internal/
    protocol/              ← was NtunlCommon (Command, DTOs)
      command.go
      dto.go
    compress/              ← gzip/brotli helpers (Utility.cs)
      compress.go
    certs/                 ← self-signed cert gen (Utility.GetOrCreateCertificate)
      certs.go
    logx/                  ← colored console logger (LogFormatter.cs)
      logx.go
    host/                  ← TunnelHost, HttpServer, message handler
      tunnel.go
      httpserver.go
    client/                ← TunnelClient, message handler, inspector
      tunnel.go
      handler.go
      inspector.go
      templates/requests.html
  configs/
    host.json              ← was NtunlHost/appsettings.json
    client.json            ← was NtunlClient/appsettings.json
  build/
    Dockerfile.host
    Dockerfile.client
  go.mod / go.sum
  README.md                (updated)
```

## 2. Dependency mapping (.NET → Go)

| Concern              | .NET today                         | Go replacement                                  |
|----------------------|------------------------------------|-------------------------------------------------|
| WebSocket server/cli | WatsonWebsocket                    | `github.com/coder/websocket` (formerly nhooyr)  |
| HTTP server          | `System.Net.HttpListener`          | `net/http`                                      |
| HTTP client          | `HttpClient` / `IHttpClientFactory`| `net/http.Client`                               |
| JSON                 | `System.Text.Json`                 | `encoding/json`                                 |
| Gzip                 | `GZipStream`                       | `compress/gzip` (stdlib)                         |
| Brotli               | `BrotliStream`                     | `github.com/andybalholm/brotli`                 |
| GUID / conversation  | `System.Guid`                      | `github.com/google/uuid`                        |
| Self-signed certs    | `CertificateRequest` / X509        | `crypto/tls`, `crypto/x509`, `crypto/rsa`       |
| HTML template        | Handlebars.Net                     | `html/template` (stdlib)                         |
| Config (json+env)    | `Host.CreateDefaultBuilder`        | `encoding/json` + small env-override layer      |
| DI + hosted services | `Microsoft.Extensions.Hosting`     | plain goroutines + `context.Context` + errgroup |
| Logging              | custom `ConsoleFormatter`          | `log/slog` with a custom colored handler        |

Only three third-party deps: `coder/websocket`, `andybalholm/brotli`,
`google/uuid`. Everything else is stdlib.

## 3. Wire protocol (simplified)

The .NET version hand-rolls binary framing (`BinaryWriter`: little-endian int32
command type + 16-byte mixed-endian GUID + 7-bit-length-prefixed UTF-8 string).
WebSocket already frames messages, so we drop all of that. **Each WebSocket
message is one JSON `Command`:**

```go
type CommandType int
const (
    CmdEcho CommandType = 1
    CmdHttpRequest    = 2
    CmdHttpResponse   = 3
    CmdNtunlInfo      = 4
)

type Command struct {
    CommandType    CommandType `json:"commandType"`
    ConversationId string      `json:"conversationId"` // uuid
    Data           string      `json:"data"`           // nested JSON payload
}
```

`Data` carries a nested-JSON `HttpRequestData` / `HttpResponseData` /
`NtunlInfo`, exactly as today. (Could be flattened later; keeping it nested
minimizes behavioral risk in the first pass.) DTOs port 1:1:

- `HttpRequestData{ headers, method, path, content []byte, contentHeaders }`
- `HttpResponseData{ statusCode, content []byte, contentHeaders, headers }`
- `NtunlInfo{ url }`

Note: Go marshals `[]byte` as base64 in JSON — same effective behavior as the
.NET byte[] serialization, so binary bodies survive the round trip.

## 4. Component-by-component port

### internal/protocol  (NtunlCommon/Data/Command.cs)
- `Command`, `CommandType`, the three DTOs, marshal/unmarshal helpers.
- Drop the `ApiJsonSerializerContext` AOT plumbing — not needed in Go.

### internal/compress  (Utility.cs)
- `Compress(data, EncodeType)` / `BrotliDecompress` / `GzipDecompress`.
- `CombineUrlPath(base, sub)` → `path.Join`-style helper preserving the
  trailing/leading-slash trim behavior.

### internal/certs  (Utility.GetOrCreateCertificate)
- Load PFX-or-generate. Go can't read .NET PFX trivially, but since it's a
  *generate-if-missing* self-signed cert, switch the on-disk format to PEM
  (`cert.pem` + `key.pem`). Generate RSA-2048, CN=localhost, 5-year validity.
- Wire into the WS server's `tls.Config` when SSL enabled.

### internal/logx  (LogFormatter.cs)
- `slog.Handler` that prints `HH:mm:ss  level: message` with the same ANSI
  colors (trace/debug cyan, info green, warn yellow, error/critical red).

### internal/host  (NtunlHost)
- **tunnel.go** (`TunnelHost`):
  - WS server via `coder/websocket` `Accept` on each connection.
  - On connect: assign a subdomain (`GetRandomName` — reserved-list-first, else
    `word+rand`), store `clientInfo{id, name, conn, writeMu}` in a
    `map[string]*clientInfo` guarded by a `sync.RWMutex`.
  - Send `NtunlInfo` with the public URL on connect; `Echo` "no subdomains" +
    disconnect when full.
  - `SendHttpRequest(req, client, timeout)`: the key concurrency piece. Replace
    the .NET `AutoResetEvent` + shared event handler with a
    `map[conversationId]chan *HttpResponseData` (guarded by a mutex). Per-client
    `writeMu` serializes socket writes (was `SemaphoreSlim WriteLock`). The read
    loop dispatches each inbound message to the waiting channel by
    `conversationId`; caller selects on the channel vs. `time.After(timeout)`.
  - Per-connection read pump goroutine (replaces `MessageReceived` event).
- **httpserver.go** (`HttpServer` + `HttpServerMessageHandler`):
  - `net/http` server on `HttpHost.Port`.
  - Resolve client from subdomain (`host.Split(".")[0]`; `localhost`/`192` →
    any client). 404 (`DefaultResponseCode`) when no client.
  - Build `HttpRequestData` from the inbound request (method, path, body,
    content headers), apply the header blacklist (exact + `prefix*` wildcard)
    and pull client IP from `IpHeaderName` (default `X-Forwarded-For`).
  - Forward via `TunnelHost.SendHttpRequest`, copy response status/headers/body
    back, propagate `Content-Encoding`.
  - Cap concurrency (was `SemaphoreSlim(5)`) — `net/http` already pools, but
    keep an explicit semaphore if we want to preserve the limit.

### internal/client  (NtunlClient)
- **tunnel.go** (`TunnelClient`): dial the host WS (`coder/websocket` Dial),
  with the connect-retry loop (5s interval, 10 retries). TLS
  `InsecureSkipVerify` driven by `AllowInvalidCertificates`. Read pump
  dispatches by `CommandType`:
  - `Echo` → log; `NtunlInfo` → store + log the URL; `HttpRequest` → handle and
    write the `HttpResponse` back.
- **handler.go** (`ClientMessageHandler`): for each `HttpRequest`, build a
  `net/http` request to `Address + path`, copy headers (special-case `Host`
  override via `HostHeader`, apply `CustomHeader`, skip `Content-Length`/
  `Content-Type` like today), 10s timeout, read response into `HttpResponseData`,
  and apply the optional URL-rewrite (decompress → regex replace
  `RewriteUrlPattern` → `NtunlInfo.Url` → recompress) for `text/html`. Append to
  an in-memory `RequestLogs` ring for the inspector.
- **inspector.go** (`HttpServer`): optional `net/http` server on `Inspector.Port`
  rendering `templates/requests.html` via `html/template` (port the Handlebars
  template — mostly `{{#each}}`/`{{var}}` → `{{range}}`/`{{.Field}}`).
- **Multiple tunnels:** the `Tunnels` array maps to N `TunnelClient`
  goroutines, all sharing one handler/inspector (as today).

### cmd/host & cmd/client
- Parse config (`configs/host.json` / `client.json`, path overridable by flag or
  `NTUNL_CONFIG` env, plus env-var overrides), build the logger, start the
  servers under an errgroup, handle SIGINT/SIGTERM for graceful shutdown
  (replaces the Generic Host lifecycle). Print the ASCII logo (Extensions.cs).

## 5. Config translation

Keep the JSON shapes nearly identical so the README examples still read true.
Replace the `Microsoft.Extensions` binding with a struct + `json.Unmarshal` and a
thin env-override pass (e.g. `NTUNL_HTTPHOST__PORT`). Map:

- Host: `TunnelHost` (HostName, Port, ClientDomain{Domain, SubDomains},
  Ssl{Enabled, AcceptInvalidCertificates}) + `HttpHost` (HostName, Port,
  Headers{BlackList, IpHeaderName}, DefaultResponseCode).
- Client: `Tunnels[]` (SslEnabled, AllowInvalidCertificates, NtunlAddress,
  Address, HostHeader, CustomHeader, RewriteUrlEnabled, RewriteUrlPattern) +
  `Inspector{Enabled, Port}`.

## 6. Docker / CI

- Two multi-stage Dockerfiles using `golang:1.23` → `gcr.io/distroless/static`
  (or `alpine`). Far smaller than the .NET runtime images.
- Update `.github/workflows/docker-image.yml` to point `file:` at the new
  Dockerfiles and `context:` at repo root; keep the `release-*` tag trigger and
  DockerHub push. Update `pr-check.yml` to `go build ./... && go vet ./... &&
  go test ./...`.

## 7. Phased execution

1. **Scaffold** — `git mv src _legacy`; `go mod init`; create dir skeleton;
   add the 3 deps. (Compiles to empty mains.)
2. **protocol + compress + certs + logx** — pure, unit-testable leaf packages.
   Add round-trip tests for Command JSON and gzip/brotli.
3. **Host** — tunnel WS server + HTTP forwarder. Manual smoke test with a
   throwaway WS client.
4. **Client** — WS dial + request replay + retry. End-to-end: client exposes a
   local `http.FileServer`, host forwards a curl through it.
5. **Inspector** — port the Handlebars template, verify the page renders logs.
6. **Config + graceful shutdown + ASCII logo** — wire `cmd/` mains.
7. **Docker + CI + README** — new images, workflows, docs.
8. **Verify parity** — run host+client locally, exercise: plain GET, POST body,
   gzip/brotli responses, subdomain routing, URL rewrite, "no subdomains left",
   client reconnect.

## 8. Risks / watch-items

- **Concurrency model is the crux.** The .NET design serializes the WS read loop
  and matches responses by a process-wide event + `AutoResetEvent`. The Go port
  must do per-conversation channel routing with a mutex-guarded map, and
  serialize writes per-connection — get this right or responses cross wires
  under concurrency. (The .NET `_syncResponseLock` lock-around-event is actually
  a bottleneck we can improve on.)
- **Header fidelity.** .NET splits "content headers" vs "headers"; Go's
  `http.Header` doesn't. Keep the two dictionaries in the DTO and decide
  placement explicitly (Content-Type/Length/Encoding → content headers).
- **`[]byte` base64 in JSON** inflates payloads ~33%. Matches current behavior;
  flag as a later optimization (binary frame or msgpack) if throughput matters.
- **Brotli**: `andybalholm/brotli` covers compress + decompress; verify level
  parity isn't required (we only re-compress after rewrite).
- **TLS/PFX**: switching cert storage to PEM is a behavior change vs. the .NET
  `.pfx`; document it. Client `AllowInvalidCertificates` → `InsecureSkipVerify`.
- The legacy `.vs/` build artifacts and `bin/obj` should not move into
  `_legacy/`; move only `src/` source (or move all and let `.gitignore` cover
  the rest).
```
