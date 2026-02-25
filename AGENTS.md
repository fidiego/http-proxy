# http-proxy — Agent Context

Interactive reverse proxy for local development. Captures, filters, and replays HTTP traffic across multiple
upstream services.

## Module

```
github.com/fidiego/http-proxy   (~/code/http-proxy)
```

## Package Map

| Package           | Purpose                                                       |
| ----------------- | ------------------------------------------------------------- |
| `cmd/http-proxy/` | Cobra CLI — flags, config loading, wiring                     |
| `pkg/proxy/`      | Core: engine, flow model, router, addon pipeline, flow store  |
| `pkg/config/`     | YAML config (`proxy.yml`) loading and `Example()` template    |
| `pkg/filter/`     | Filter expression parser (`~m`, `~s`, `~p`, `~h`, `~b`, `~u`) |
| `pkg/addons/`     | Built-in addons: `LogAddon`, `CaptureAddon`                   |
| `pkg/tui/`        | Bubbletea terminal UI (flow list, detail view, filter input)  |
| `pkg/web/`        | Web server: REST API, WebSocket hub, embedded HTML/JS UI      |

## Core Concepts

### Flow

`pkg/proxy/flow.go` — central data model for one HTTP transaction.

```go
type Flow struct {
    ID        string
    Upstream  string          // upstream name that handled this flow
    Request   *CapturedRequest
    Response  *CapturedResponse
    Error     error
    State     FlowState       // active | intercepted | complete | error
    Tags      []string
    // ...
}
```

Flows are tagged automatically (`replay`, `replay:<original-id>` for replayed flows).

### FlowStore

`pkg/proxy/flow_store.go` — thread-safe ring buffer with pub/sub.

- `Add`, `Get`, `All`, `Count`, `Clear`
- `Subscribe() <-chan FlowEvent` / `Unsubscribe(ch)`
- Slow subscribers have events dropped (non-blocking send)

### Addon Pipeline

`pkg/proxy/addon.go` — hook-based plugin system.

```go
type RequestHook  interface { OnRequest(ctx, flow)  }
type ResponseHook interface { OnResponse(ctx, flow) }
type CompleteHook interface { OnComplete(ctx, flow) }
type ErrorHook    interface { OnError(ctx, flow)    }
```

Addons implement only the hooks they need. Register with `engine.Addons().Add(addon)`.

### Router

`pkg/proxy/router.go` — longest-prefix-first path routing.

Each upstream has a `Prefix` (e.g. `/api`). The router sorts by descending prefix length and returns the first match. A
`/` catch-all is typical.

### Engine

`pkg/proxy/engine.go` — wires together router, per-upstream `httputil.ReverseProxy` instances, addon pipeline, and flow
store.

Key methods:

- `New(opts Options) (*Engine, error)`
- `Start(ctx context.Context) error` — starts the HTTP listener
- `Replay(flowID string) error` — replays a captured request through the pipeline
- `Store() *FlowStore`
- `Addons() *AddonManager`
- `Options() Options`

Body capture uses `io.LimitReader` (default 1 MiB). The full body is still forwarded to the upstream/client — only the
captured copy is truncated.

### Config

`pkg/config/config.go` — YAML config with pointer fields for optional integers.

Loading priority: defaults → config file → explicit CLI flags (`cmd.Flags().Changed()`).

Auto-discovered filenames: `proxy.yml`, `proxy.yaml`, `.proxy.yml`.

### Filter Language

`pkg/filter/filter.go` — recursive-descent parser.

```
~m METHOD    method contains (case-insensitive)
~s CODE      status prefix ("5" → all 5xx)
~p PATH      path contains
~h KEY:VAL   header key+value substring
~b TEXT      request or response body substring
~u NAME      upstream name substring

Combinators: ! & | ()
```

## Default Ports

| Component         | Default                  |
| ----------------- | ------------------------ |
| Proxy listener    | `:9090`                  |
| Web UI / REST API | `9091`                   |
| WebSocket         | `ws://localhost:9091/ws` |

## Building

```sh
go build ./...              # build all packages
go build -o /tmp/http-proxy ./cmd/http-proxy  # build binary
go vet ./...
```

The project uses Go 1.25+. Always run `go fmt` after editing Go files.

## Extending

### Custom addon

```go
type MyAddon struct{}

func (a *MyAddon) OnComplete(ctx context.Context, flow *proxy.Flow) {
    // inspect or mutate flow after response
}
```

Register: `engine.Addons().Add(&MyAddon{})`.

### Using as a library

```go
import (
    "github.com/fidiego/http-proxy/pkg/proxy"
    "github.com/fidiego/http-proxy/pkg/web"
)

engine, _ := proxy.New(proxy.Options{
    ListenAddr: ":9090",
    WebPort:    9091,
    Upstreams:  []proxy.Upstream{
        {Name: "ctl-api", Prefix: "/", Target: "http://localhost:8081"},
    },
})
engine.Addons().Add(myAddon)
webSrv := web.New(engine, 9091)

g, ctx := errgroup.WithContext(ctx)
g.Go(func() error { return engine.Start(ctx) })
g.Go(func() error { return webSrv.Start(ctx) })
g.Wait()
```
