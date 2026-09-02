---
title: "Getting Started"
weight: 2
---

# Getting Started

This walkthrough takes you from zero to a running event-reactor that reacts to
an event in a few minutes. No prior knowledge is assumed.

event-reactor does three things: it **listens** for events over HTTP, **matches**
them against [CEL](https://github.com/google/cel-go) expressions, and **reacts**
by calling a provider (log, HTTP, exec, echo).

## 1. Write a minimal config

Create a file named `config.yaml`:

```yaml
apiVersion: event-reactor.io/v1
kind: ServerConfig

server:
  port: 8080

observability:
  logging:
    level: info
    format: text

reactors:
  - name: log-opened-prs
    # CEL expression evaluated against every incoming event.
    match: payload.action == "opened"
    provider: log
    inputs:
      level: info
      message:
        template: "PR #{{ .payload.number }} opened in {{ .payload.repository.full_name }}"
```

This says: for any event whose payload has `action == "opened"`, write a line to
the server log.

## 2. Run the server

Using the published container image (the default command reads
`/config/server.yaml`):

```bash
docker run --rm -p 8080:8080 \
  -v "$PWD/config.yaml:/config/server.yaml:ro" \
  ghcr.io/oakwood-commons/event-reactor:latest
```

Or with a locally built binary:

```bash
task build
dist/er run server --config config.yaml
```

The server logs `starting HTTP server` on `:8080` once ready.

## 3. Send an event

In another terminal, POST a JSON event to the generic `/events` endpoint:

```bash
curl -sS -X POST http://localhost:8080/events \
  -H 'Content-Type: application/json' \
  -d '{"action":"opened","number":42,"repository":{"full_name":"acme/widgets"}}'
```

The endpoint responds with the number of reactors that fired:

```json
{"processed":1,"results":[{"provider":"log","output":"..."}]}
```

and the server log shows:

```
PR #42 opened in acme/widgets
```

Send an event that does not match (`"action":"closed"`) and `processed` will be
`0`.

## 4. Validate before you deploy

You can test configs and expressions locally without starting the server:

```bash
# Validate the config file and compile all CEL expressions
dist/er test config config.yaml

# Check a CEL expression against a sample event JSON file
echo '{"action":"opened","number":42}' > event.json
dist/er test match 'payload.action == "opened"' -e event.json   # prints MATCH

# Render a template against the same event
dist/er test template -t 'PR #{{ .payload.number }}' -e event.json
```

> Note: for `er test`, the top-level JSON object in the event file becomes the
> event `payload`. See the [CLI reference](../cli/) for details.

## Next steps

- [API reference](../api/) -- endpoints, request formats, and the event model
  you match on.
- [Configuration](../design/configuration/) -- full config schema, input
  resolution, listeners, and hot-reload.
- [Providers](../design/providers/) -- log, http, exec, echo, and writing your
  own.
- [Auth](../design/auth/) -- outbound token handlers and inbound webhook
  signature validation.
- [Operations](../operations/) -- deployment, health checks, and verifying
  signed artifacts.
