---
title: "CLI Reference"
weight: 4
---

# CLI Reference

The `er` binary is the single entry point for running the server and for testing
configs, expressions, templates, and reactors locally.

```bash
er [command]
```

| Command | Purpose |
|---------|---------|
| `er version` | Print version, commit, and build time. |
| `er run server` | Start the HTTP server. |
| `er test match` | Evaluate a CEL expression against an event file. |
| `er test template` | Render a Go template against an event file. |
| `er test config` | Validate a server config and compile its CEL expressions. |
| `er test reactor` | Dry-run or inspect a single reactor against an event. |
| `er mcp` | Start the MCP (Model Context Protocol) server over stdio. |

## `er version`

Prints the build version string, for example
`event-reactor v1.2.3 (commit: abc1234, built: 2025-01-01T00:00:00Z)`.

## `er run server`

Starts the HTTP server and any background listeners defined in the config.

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--config` | `-c` | yes | Path to the server config YAML. |

```bash
er run server --config config.yaml
```

The server binds `server.port` (default `8080`), pre-compiles every reactor
`match` expression at startup (failing fast on invalid CEL), and shuts down
gracefully on `SIGINT`/`SIGTERM`.

## `er test` subcommands

The `test` subcommands let you validate behavior without sending real traffic.

For `test match`, `test template`, and `test reactor`, the event is loaded from a
JSON file where the **top-level object becomes the event `payload`**. Include a
top-level `attributes` object to populate event attributes.

### `er test match`

Evaluate a CEL expression and print `MATCH` or `NO MATCH`.

```bash
er test match 'payload.action == "opened"' --event event.json
```

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--event` | `-e` | yes | Path to the event JSON file. |

### `er test template`

Render a Go template against the event and print the result.

```bash
er test template --template 'PR #{{ .payload.number }}' --event event.json
er test template --file message.tmpl --event event.json
```

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--event` | `-e` | yes | Path to the event JSON file. |
| `--template` | `-t` | one of `-t`/`-f` | Inline template string. |
| `--file` | `-f` | one of `-t`/`-f` | Path to a template file. |

### `er test config`

Load and validate a config file, then compile all reactor CEL expressions.
Reports the listener and reactor counts and any invalid expression.

```bash
er test config config.yaml
```

### `er test reactor`

Check whether an event matches a named reactor and resolve its inputs. By default
this is a dry-run (`--dry-run=true`) that prints the resolved inputs as JSON
without calling the provider.

```bash
er test reactor --config config.yaml --event event.json --name log-opened-prs
```

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--config` | `-c` | yes | Path to the server config YAML. |
| `--name` | `-n` | yes | Reactor name to test. |
| `--event` | `-e` | yes | Path to the event JSON file. |
| `--dry-run` | | no | Resolve inputs without executing (default `true`). |

## `er mcp`

Starts an MCP server over stdio for use by AI agents and MCP-aware clients.
Logs go to stderr; the JSON-RPC protocol uses stdin/stdout.

```bash
er mcp
```
