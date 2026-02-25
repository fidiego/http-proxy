# http-proxy

An interactive reverse proxy for local development. Inspired by mitmproxy and ngrok's traffic inspector.

Captures, filters, and replays HTTP traffic across multiple upstream services from a single listening port.

## Features

- **Multi-upstream routing** — path-prefix routing to any number of backends
- **Interactive TUI** — real-time flow list, detail view, filter, replay (bubbletea)
- **Web UI** — browser-based inspector with WebSocket streaming on `localhost:9091`
- **Filter expressions** — `~m`, `~s`, `~p`, `~h`, `~b`, `~u` with `!`, `&`, `|`, `()`
- **Replay** — resend any captured request through the proxy pipeline
- **Copy as cURL** — one-keystroke cURL export from the TUI
- **YAML config** — `proxy.yml` auto-discovered in CWD; CLI flags override

## Quick Start

```sh
# Build
go build -o http-proxy ./cmd/http-proxy

# Single upstream
./http-proxy --upstream http://localhost:8081

# Multiple upstreams with path routing
./http-proxy \
  --route /api=http://localhost:8081 \
  --route /runner=http://localhost:8083 \
  --route /=http://localhost:4000

# From a config file
./http-proxy --config proxy.yml

# Generate an example config
./http-proxy init > proxy.yml
```

## Config File

`proxy.yml` (or `proxy.yaml`, `.proxy.yml`) is loaded automatically from the current directory.

```yaml
listen: ':9090'
web_port: 9091
no_tui: false
no_color: false
max_flows: 1000

upstreams:
  - name: ctl-api
    prefix: /api
    target: http://localhost:8081
  - name: runner
    prefix: /runner
    target: http://localhost:8083
  - name: dashboard
    prefix: /
    target: http://localhost:4000
```

Priority: defaults → config file → explicit CLI flags.

## TUI Key Bindings

| Key       | Action                     |
| --------- | -------------------------- |
| `j` / `k` | Move down / up             |
| `Enter`   | Open flow detail           |
| `Esc`     | Back to list               |
| `f`       | Focus filter input         |
| `r`       | Replay selected flow       |
| `c`       | Copy selected flow as cURL |
| `d`       | Clear all flows            |
| `q`       | Quit                       |

## Filter Expression Language

Expressions can be combined with `!`, `&`, `|`, and `()`.

| Token                  | Matches                               |
| ---------------------- | ------------------------------------- |
| `~m GET`               | HTTP method contains `GET`            |
| `~s 5`                 | Status code starts with `5` (all 5xx) |
| `~p /api`              | URL path contains `/api`              |
| `~h content-type:json` | Header key/value substring            |
| `~b error`             | Request or response body substring    |
| `~u ctl-api`           | Upstream name substring               |

Examples:

```
~s 5
~m POST & ~p /api
~s 4 | ~s 5
!~m GET & ~p /api
```

## Web UI

Available at `http://localhost:9091` (default) while the proxy is running.

- Real-time flow stream via WebSocket
- Master-detail layout with request/response inspection
- Client-side filter bar
- HAR export, replay, copy as cURL

REST API:

```
GET    /api/flows          list all captured flows
GET    /api/flows/{id}     get a specific flow
POST   /api/flows/{id}/replay  replay a flow
DELETE /api/flows          clear all flows
GET    /api/config         current proxy config
GET    /ws                 WebSocket stream of flow events
```

## Package Structure

```
cmd/http-proxy/   CLI entry point (cobra)
pkg/proxy/        core engine, flow model, router, addon pipeline
pkg/config/       YAML config loading
pkg/filter/       filter expression parser
pkg/addons/       built-in addons (log, capture)
pkg/tui/          bubbletea terminal UI
pkg/web/          web server, REST API, embedded HTML UI
```

## Embedding as a library

The `pkg/proxy` package is designed for library use:

```go
engine, err := proxy.New(proxy.Options{
    ListenAddr: ":9090",
    WebPort:    9091,
    Upstreams: []proxy.Upstream{
        {Name: "api", Prefix: "/api", Target: "http://localhost:8081"},
    },
})
engine.Addons().Add(myAddon)
engine.Start(ctx)
```
