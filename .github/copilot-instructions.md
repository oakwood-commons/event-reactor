# event-reactor - AI Agent Instructions

## Overview

Go-based event-driven automation engine. Listens for events (HTTP push, CloudEvents, webhooks), matches them with CEL expressions, and dispatches reactions via pluggable providers (echo, http, exec, log). Supports named auth handlers for outbound token injection, Go template rendering, config hot-reload, rate limiting, and circuit breaking.

**Repository**: [oakwood-commons/event-reactor](https://github.com/oakwood-commons/event-reactor)

## Architecture

```
event-reactor (this repo)
  cmd/er/                    -- CLI entry point
  pkg/adapter/               -- event fan-out: listener -> matcher -> reactor
  pkg/api/                   -- HTTP API server (Gin): /events, /cloudevents, /webhook/:source
  pkg/auth/                  -- auth handler registry and token generation
  pkg/circuitbreaker/        -- circuit breaker for reactor fault isolation
  pkg/cmd/                   -- CLI command wiring (Cobra)
  pkg/config/                -- server config loading/validation from YAML
  pkg/listener/              -- Listener interface
  pkg/listener/generic/      -- generic HTTP push listener
  pkg/listener/pubsub/       -- GCP Pub/Sub listener (stub)
  pkg/matcher/               -- CEL expression compilation and matching
  pkg/mcp/                   -- MCP server (Model Context Protocol, stdio JSON-RPC)
  pkg/message/               -- Event type and normalization
  pkg/observability/         -- structured logging (slog) factory
  pkg/params/version/        -- version info (ldflags)
  pkg/ratelimit/             -- token-bucket rate limiter
  pkg/reactor/               -- Provider interface, registry, input resolution
  pkg/reactor/providers/     -- built-in providers (echo, http, exec, log)
  pkg/reload/                -- config file hot-reload via fsnotify
  pkg/template/              -- Go template rendering with custom functions
  deploy/                    -- Kustomize manifests (base + production)
  test/                      -- integration test helpers and test data
```

### Key Concepts

- **Listener** -- listens for incoming events (HTTP push, CloudEvents, webhooks)
- **Matcher** -- filters events using CEL expressions against attributes/payload
- **Reactor** -- executes a response (provider call) when an event matches
- **Provider** -- pluggable execution backend (echo, http, exec, log)
- **Adapter** -- bridges listeners to matchers and reactors with auth injection
- **Auth Handler** -- named token generator (static, GitHub App/PAT, OAuth2, GCP SA)
- **Config** -- YAML server config defining listeners, reactors, and auth

## Key Patterns

### CEL Expressions

Event filtering uses CEL via `pkg/matcher/`:
- Compile and cache CEL programs (sync.RWMutex)
- Evaluate expressions against event payload, attributes, id, source, type
- Used in reactor `match` field

### Go Templates

Use `pkg/template/` for input rendering:
- Custom functions: upper, lower, trimSpace, contains, hasPrefix, hasSuffix, replace, split, join, trimPrefix, trimSuffix, default, toJSON
- Used in reactor input resolution (template inputs)

### Auth Handlers

Use `pkg/auth/` for outbound token generation:
- Named handlers configured in YAML (`auth.handlers[]`)
- Reactors reference by name (`auth: handler-name`)
- Token injected as `Authorization` header on provider calls
- Types: static-token, github-token, github-app, oauth2-client-credentials, service-account

### Input Resolution

Reactor inputs resolved via `pkg/reactor/` (priority order):
1. `valueFrom.secretKeyRef` -- GCP Secret Manager
2. `payloadValue.propertyPaths` -- CEL path extraction
3. `fromFile` -- read from file
4. `fromEnv` -- environment variable
5. `expr` -- CEL expression evaluation
6. `template` -- Go template rendering
7. scalar -- static value

### Configuration

Server config loaded from YAML via `pkg/config/`:
- Defines server settings, observability, auth handlers, listeners, reactors
- Hot-reload via `pkg/reload/` (fsnotify)

## Conventions

- **Commits**: Use [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/#specification)
- **Signing**: All commits must be GPG/SSH signed (`-S`) and include DCO sign-off (`-s`)
- **Errors**: Return errors with `fmt.Errorf("context: %w", err)`, don't panic
- **License**: Apache License 2.0

## Build & Test Commands

```bash
# Build
task build

# Run server
dist/er run server --config config.yaml

# Test
task test                        # Run all unit tests
task coverage                    # Coverage report

# Smoke test (starts server, sends events, validates responses)
pwsh scripts/helper.ps1

# Lint & vet
task lint
task vet
```

## Security Scanning

```bash
task vuln
```

## Critical Rules

- **Business logic placement**: Keep domain logic in dedicated `pkg/` packages, not in CLI command wiring
- **After any change**: Run `go test ./...` to ensure everything passes
- **No magic values**: Always define constants for configuration values
- **Git safety**: Never run `git commit`, `git push`, or `git commit --amend` unless the user explicitly asks

## Additional Conventions

Go coding conventions (error handling, design patterns), testing rules, PowerShell scripting conventions, and documentation requirements are in `.github/instructions/*.instructions.md` files -- they load automatically when editing relevant files.