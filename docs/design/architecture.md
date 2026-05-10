---
title: "Architecture"
weight: 1
---

# Architecture

event-reactor is a single Go binary that receives events, evaluates CEL filter expressions, and dispatches matched events to provider-based reactors.

## System Overview

```
+--------------------------------------------------------------------+
|                         event-reactor                               |
|                                                                     |
|  +----------+    +----------+    +----------+    +--------------+   |
|  | Listener |--->| Adapter  |--->| Matcher  |--->|   Reactor    |   |
|  |          |    | (fan-out)|    |  (CEL)   |    |  (provider)  |   |
|  +----------+    +----------+    +----------+    +--------------+   |
|       ^                                                |            |
|       |                                                v            |
|  +----------+                                   +--------------+   |
|  |  HTTP    |                                   | Auth Handler |   |
|  |  Server  |                                   | (token gen)  |   |
|  +----------+                                   +--------------+   |
+--------------------------------------------------------------------+
```

## Event Flow

1. **Ingestion** -- Events arrive via HTTP endpoints (`/events`, `/cloudevents`, `/webhook/:source`)
2. **Normalization** -- Raw payloads are normalized into an `Event` struct (ID, Source, Type, Attributes, Payload)
3. **Fan-out** -- The adapter iterates all configured reactors concurrently
4. **Matching** -- Each reactor's CEL expression is evaluated against the event
5. **Input Resolution** -- Matched reactors resolve their inputs (template, expr, env, file, secret, payload extraction)
6. **Auth Injection** -- If the reactor references an auth handler, a token is generated and injected
7. **Execution** -- The provider executes with resolved inputs (HTTP call, command, log, etc.)

## Package Layout

| Package | Responsibility |
|---------|---------------|
| `cmd/er` | CLI entry point |
| `pkg/adapter` | Event fan-out: listener -> matcher -> reactor |
| `pkg/api` | HTTP server (Gin): health, events, CloudEvents, webhooks |
| `pkg/auth` | Auth handler registry, token generation |
| `pkg/circuitbreaker` | Circuit breaker for reactor fault isolation |
| `pkg/cmd` | CLI command wiring (Cobra) |
| `pkg/config` | YAML config loading, validation, defaults |
| `pkg/listener` | Listener interface |
| `pkg/listener/generic` | Generic HTTP push listener |
| `pkg/matcher` | CEL compilation, caching, evaluation |
| `pkg/mcp` | MCP server (stdio JSON-RPC) for AI tooling |
| `pkg/message` | Event struct, normalization helpers |
| `pkg/observability` | Structured logging (slog) factory |
| `pkg/ratelimit` | Token-bucket rate limiter |
| `pkg/reactor` | Provider interface, registry, input resolution |
| `pkg/reactor/providers` | Built-in providers (echo, http, exec, log) |
| `pkg/reload` | Config hot-reload via fsnotify |
| `pkg/template` | Go template rendering with custom functions |

## Design Principles

- **Single binary** -- One static binary, no runtime dependencies
- **Config-driven** -- All behavior defined in YAML, no code changes needed
- **Pluggable providers** -- New reactions added by implementing one interface
- **Concurrent execution** -- Matched reactors run in parallel goroutines
- **Secure by default** -- Auth tokens are never logged, webhook signatures validated, distroless container
- **Observable** -- Structured logging, health endpoints, metrics-ready

## Threading Model

- The HTTP server handles requests concurrently (Gin's goroutine-per-request)
- For each event, the adapter spawns one goroutine per matching reactor
- CEL program compilation is cached with a read-write mutex
- Auth token generation is thread-safe (per-handler mutex)
- The rate limiter and circuit breaker use fine-grained mutexes

## Configuration Hot-Reload

The server watches the config file via fsnotify. On change:
1. New config is parsed and validated
2. CEL expressions are re-compiled
3. The adapter atomically swaps to the new config
4. Running requests complete with the old config (no interruption)
