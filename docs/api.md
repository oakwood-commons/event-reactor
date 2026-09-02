---
title: "API Reference"
weight: 3
---

# API Reference

event-reactor exposes a small HTTP API for ingesting events plus health
endpoints. All ingestion endpoints accept a JSON body, normalize it into a
common [event model](#event-model), run it through the matcher and reactors, and
return a summary of what fired.

The server listens on `server.port` (default `8080`). Health endpoint paths are
configurable under `server.healthCheck`.

## Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/health/live` | Liveness probe (path from `server.healthCheck.liveness`) |
| `GET` | `/health/ready` | Readiness probe (path from `server.healthCheck.readiness`) |
| `POST` | `/events` | Generic JSON event ingestion (auto-detects Pub/Sub push) |
| `POST` | `/cloudevents` | CloudEvents ingestion (structured or binary content mode) |
| `POST` | `/webhook/:source` | Webhook ingestion with optional HMAC validation |

### Ingestion response

All ingestion endpoints return `200 OK` with the number of reactors that
matched and executed:

```json
{
  "processed": 1,
  "results": [
    { "provider": "log", "output": "..." }
  ]
}
```

`processed` is `0` when no reactor matches. Malformed JSON returns `400`.

## `POST /events`

Generic ingestion. The request body must be a JSON object; it becomes the event
`payload`.

```bash
curl -sS -X POST http://localhost:8080/events \
  -H 'Content-Type: application/json' \
  -d '{"action":"opened","number":42}'
```

If the body is a GCP Pub/Sub push envelope (an object with a `message` field that
contains `data`), it is decoded as a Pub/Sub message: `message.data` is parsed as
JSON (raw or base64-encoded) into the payload, and `message.attributes` become
event attributes. CloudEvents attributes carried as `ce-*` keys map onto the
event `id`, `source`, and `type`.

```json
{
  "message": {
    "data": "eyJhY3Rpb24iOiJvcGVuZWQifQ==",
    "attributes": { "ce-type": "com.example.pr.opened" },
    "messageId": "123"
  }
}
```

## `POST /cloudevents`

Accepts CloudEvents in either content mode.

**Structured mode** -- the full CloudEvent is the JSON body. `data` becomes the
payload; `id`, `source`, `type`, and `specversion` populate the event and its
attributes. If there is no `data` field, the whole body is used as the payload.

```bash
curl -sS -X POST http://localhost:8080/cloudevents \
  -H 'Content-Type: application/json' \
  -d '{
        "specversion": "1.0",
        "id": "abc-123",
        "source": "/my/service",
        "type": "com.example.pr.opened",
        "data": {"action":"opened","number":42}
      }'
```

**Binary mode** -- metadata is carried in `Ce-*` headers and the body is the
data. Detected when `Ce-Type` is present. A missing `Ce-Id` is filled with a
generated UUID.

```bash
curl -sS -X POST http://localhost:8080/cloudevents \
  -H 'Content-Type: application/json' \
  -H 'Ce-Specversion: 1.0' \
  -H 'Ce-Id: abc-123' \
  -H 'Ce-Source: /my/service' \
  -H 'Ce-Type: com.example.pr.opened' \
  -d '{"action":"opened","number":42}'
```

## `POST /webhook/:source`

Ingests a webhook. The `:source` path segment names the sender (for example
`/webhook/github`) and is stored as the event `source`. The full JSON body
becomes the payload.

```bash
curl -sS -X POST http://localhost:8080/webhook/github \
  -H 'Content-Type: application/json' \
  -H 'X-GitHub-Event: pull_request' \
  -d '{"action":"opened","number":42}'
```

Event `type` resolution: the `X-Event-Type` header is used first, falling back to
`X-GitHub-Event`.

**Signature validation.** If `auth.webhookSecrets` contains an entry whose
`source` equals the `:source` segment, the request must carry a valid
HMAC-SHA256 signature. The handler reads `X-Hub-Signature-256` (falling back to
`X-Signature-256`), strips an optional `sha256=` prefix, and compares it against
the HMAC of the raw body. Invalid or missing signatures return `401`.

```yaml
auth:
  webhookSecrets:
    - source: github        # matches POST /webhook/github
      secret: whsec_abc123
```

Sources without a configured secret are accepted without signature checks.

## Event model

Every listener and endpoint normalizes input into a single `Event` envelope:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique event identifier (from the source, or a generated UUID). |
| `source` | string | Where the event originated (e.g. the webhook `:source`, `pubsub`). |
| `type` | string | Event classification (e.g. `com.example.pr.opened`). |
| `time` | timestamp | When the event was produced. |
| `attributes` | map[string]string | Metadata extracted from the transport (headers, Pub/Sub attributes). |
| `payload` | any | The decoded body -- usually a JSON object. |

## CEL variables

Reactor `match` expressions and `expr` inputs are evaluated against these
top-level variables:

| Variable | Type | Example |
|----------|------|---------|
| `payload` | any | `payload.action == "opened"` |
| `attributes` | map | `attributes["type"] == "pull_request"` |
| `id` | string | `id != ""` |
| `source` | string | `source == "github"` |
| `type` | string | `type == "com.example.pr.opened"` |

Go templates in `template` inputs receive the same keys via the event map, so
`{{ .payload.number }}`, `{{ .source }}`, and `{{ .type }}` are all available.

See [Configuration](../design/configuration/) for how reactors consume these,
and [Providers](../design/providers/) for what each provider does with the
resolved inputs.
