# event-reactor - AI Agent Instructions

## Overview

Go-based event-driven API and CLI for listening to and reacting to events from Google Cloud Pub/Sub (and other sources). Uses CEL for event filtering, Go templates for message transformation, and a pluggable reactor system for dispatching responses (email, GitHub comments, webhooks, Webex, PowerShell, etc.).

**Repository**: [oakwood-commons/event-reactor](https://github.com/oakwood-commons/event-reactor)

## Architecture

```
event-reactor (this repo)
  cmd/er/               -- CLI entry point
  cmd/gen-docs/         -- documentation generator
  pkg/adapter/          -- event adapter (Pub/Sub message handling)
  pkg/api/              -- HTTP API server (Gin-based)
  pkg/cel/              -- CEL expression evaluation for event filtering
  pkg/cli/              -- CLI helpers (IOStreams, color)
  pkg/cmd/              -- CLI command wiring (Cobra)
  pkg/config/           -- server configuration loading
  pkg/email/            -- email utilities
  pkg/encoding/         -- encoding helpers (UTF-16)
  pkg/gcp/              -- GCP clients (Pub/Sub, Secret Manager)
  pkg/github/           -- GitHub API client
  pkg/http/             -- HTTP client, caching, webhooks
  pkg/listener/         -- event listener interface and implementations
  pkg/logger/           -- structured logging (zap)
  pkg/maps/             -- map utilities
  pkg/matcher/          -- event matching logic (CEL-based)
  pkg/message/          -- message types and Pub/Sub message handling
  pkg/params/           -- CLI parameters, settings, version info
  pkg/password/         -- password utilities
  pkg/pwsh/             -- PowerShell execution
  pkg/reactor/          -- reactor interface and implementations
  pkg/template/         -- Go template rendering with custom functions
  pkg/webex/            -- Webex messaging client
  test/                 -- integration test helpers and test data
```

### Key Concepts

- **Listener** -- listens for incoming events (e.g., GCP Pub/Sub subscription)
- **Matcher** -- filters events using CEL expressions against attributes/payload
- **Reactor** -- executes a response to a matched event (email, webhook, GitHub comment, etc.)
- **Adapter** -- bridges listeners to matchers and reactors
- **Config** -- server configuration defining listeners, matchers, and reactor bindings

## Key Patterns

### CLI Output

Use `IOStreams` from `pkg/cli/` for terminal output:
- `ioStreams.Out` for standard output
- `ioStreams.ErrOut` for error output
- `ioStreams.ColorScheme()` for colored output
- Respects `--quiet` and `--no-color` flags

### CEL Expressions

Event filtering uses CEL via `pkg/cel/`:
- Evaluate expressions against event attributes and payload
- Used in server config to match incoming events to reactors

### Go Templates

Use `pkg/template/` for message rendering:
- Custom template functions available for string manipulation
- Used in reactor properties (email subjects/bodies, GitHub comments, etc.)

### HTTP Client

Use `pkg/http/` for outbound HTTP:
- Retryable HTTP client via `hashicorp/go-retryablehttp`
- HTTP caching support
- Webhook delivery

### Configuration

Server config is loaded from YAML/JSON files via `pkg/config/`:
- Defines listeners, matchers, and reactor bindings
- Supports property resolution from static values, env vars, files, GCP secrets

## Conventions

- **Commits**: Use [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/#specification)
- **Signing**: All commits must be GPG/SSH signed (`-S`) and include DCO sign-off (`-s`)
- **Errors**: Return errors with `fmt.Errorf("context: %w", err)`, don't panic
- **License**: Apache License 2.0

## Build & Test Commands

```bash
# Build
task build

# Run API server
task api-server

# Test
task test                        # Run all unit tests
task test:race                   # Run with race detector
task test:e2e                    # Run e2e tests

# Lint & vet
task lint
task vet

# Generate docs
task docs
```

## Security Scanning

```bash
task security:scan
task vuln
```

## Critical Rules

- **Business logic placement**: Keep domain logic in dedicated `pkg/` packages, not in CLI command wiring
- **After any change**: Run `go test ./...` to ensure everything passes
- **No magic values**: Always define constants for configuration values
- **Git safety**: Never run `git commit`, `git push`, or `git commit --amend` unless the user explicitly asks

## Additional Conventions

Go coding conventions (error handling, design patterns), testing rules, and documentation requirements are in `.github/instructions/*.instructions.md` files -- they load automatically when editing relevant files.