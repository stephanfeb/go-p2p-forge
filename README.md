# go-p2p-forge

A Netty-inspired Go framework for building performant libp2p server applications.

go-p2p-forge extracts common patterns from building production libp2p services into a reusable toolkit — middleware pipelines, buffer pooling, structured lifecycle management, and a server builder that gets you from zero to a working P2P server in minutes.

## Features

- **Middleware Pipeline** — Composable handler chain with bidirectional processing (inspired by Netty's ChannelPipeline). Each middleware wraps the next, enabling both request pre-processing and response post-processing in a single function.
- **Buffer Pooling** — Tiered `sync.Pool` buffer management (4KB / 64KB / 1MB) for frame I/O, reducing GC pressure under high throughput.
- **Server Builder** — Option-pattern server construction (inspired by Netty's ServerBootstrap) with sensible defaults for identity, DHT, GossipSub, relay, and transport.
- **Rate Limiting** — Per-peer sliding window rate limiter with single-bucket and dual-bucket (read/write) variants, usable as standalone or as pipeline middleware.
- **Service Lifecycle** — Ordered startup with rollback-on-failure, reverse-order shutdown, and a `TickerService` helper for periodic background tasks.
- **P2P Stack** — Configurable libp2p host creation (pluggable transports, Noise security, Yamux muxer, relay, AutoNAT) and a DHT + GossipSub node wrapper with topic management.
- **Test Helpers** — `MockStream` and `MockConn` for testing pipelines and middleware without a real libp2p network.

## Quick Start

```go
package main

import (
    "context"
    "log/slog"
    "os"
    "os/signal"

    forge "github.com/stephanfeb/go-p2p-forge"
    "github.com/stephanfeb/go-p2p-forge/codec"
    "github.com/stephanfeb/go-p2p-forge/middleware"
)

type PingRequest struct {
    Message string `json:"message"`
}

type PingResponse struct {
    Reply string `json:"reply"`
}

func main() {
    logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
    pool := codec.NewBufferPool()
    limiter := middleware.NewSingleBucket(time.Minute, 100)

    // Build a protocol pipeline
    pipeline := forge.NewPipeline(logger,
        middleware.Recovery(),
        forge.JSONResponseWriter(),
        forge.FrameDecodeMiddleware(pool),
        middleware.RateLimitMiddleware(limiter),
        forge.JSONDeserialize[PingRequest](),
        func(sc *forge.StreamContext, next func()) {
            req := sc.Request.(*PingRequest)
            sc.Response = &PingResponse{Reply: "pong: " + req.Message}
        },
    )

    // Build and start the server
    srv := forge.NewServer(
        forge.WithPort(9000),
        forge.WithLogger(logger),
    )
    srv.Handle("/my-app/ping/1.0.0", pipeline.StreamHandler())

    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()

    if err := srv.ListenAndServe(ctx); err != nil {
        logger.Error("server error", "error", err)
    }
}
```

## Architecture

```
                    Inbound                          Outbound
                    ───────►                         ◄───────

              ┌──────────────┐
              │   Recovery   │  catches panics
              └──────┬───────┘
                     │
              ┌──────▼───────┐
              │  Response    │  marshals sc.Response after handler returns
              │  Writer      │
              └──────┬───────┘
                     │
              ┌──────▼───────┐
              │  Frame       │  reads length-prefixed frame into sc.RawBytes
              │  Decode      │  (pooled buffers)
              └──────┬───────┘
                     │
              ┌──────▼───────┐
              │  Rate        │  per-peer sliding window check
              │  Limiter     │
              └──────┬───────┘
                     │
              ┌──────▼───────┐
              │  JSON        │  unmarshals sc.RawBytes → sc.Request
              │  Deserialize │
              └──────┬───────┘
                     │
              ┌──────▼───────┐
              │  Handler     │  your business logic
              │  (app code)  │  sets sc.Response
              └──────────────┘
```

Each middleware calls `next()` to proceed. Code before `next()` runs on the inbound path; code after runs on the outbound path. Not calling `next()` short-circuits the pipeline (e.g., rate limit rejection).

## Packages

| Package | Description |
|---|---|
| `forge` (root) | `Server`, `Pipeline`, `StreamContext`, `Middleware`, codec middleware, config |
| `codec` | Length-prefixed frame codec, tiered buffer pool |
| `middleware` | Rate limiter, panic recovery, pipeline middleware adapters |
| `service` | `Lifecycle` manager, `TickerService` for periodic tasks |
| `host` | libp2p host creation, Ed25519 identity management |
| `node` | DHT + GossipSub wrapper, topic management |
| `forgetest` | `MockStream`, `MockConn`, `GenerateTestPeerID()` |

## Testing

```bash
go test ./...
```

All packages include tests. Use `forgetest.MockStream` to test your pipelines without a network:

```go
func TestMyHandler(t *testing.T) {
    reqData, _ := json.Marshal(MyRequest{Value: 42})
    stream := forgetest.NewMockStreamWithFrame(forgetest.GenerateTestPeerID(), reqData)

    pipeline.HandleStream(stream)

    respData, _ := stream.ReadResponseFrame()
    var resp MyResponse
    json.Unmarshal(respData, &resp)
    // assert on resp...
}
```

## License

MIT
