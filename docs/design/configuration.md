---
title: "Configuration"
weight: 4
---

# Configuration

event-reactor is configured via a single YAML file passed to `er run server --config <path>`.

## Full Example

```yaml
apiVersion: event-reactor.io/v1
kind: ServerConfig

server:
  port: 8080
  metricsPort: 9090
  healthCheck:
    liveness: /health/live
    readiness: /health/ready

observability:
  logging:
    level: info       # debug, info, warn, error
    format: json      # json, text
  tracing:
    enabled: true
    exporter: otlp
    endpoint: localhost:4317
    sampleRate: 0.1
  metrics:
    enabled: true
    prometheus: true

auth:
  handlers:
    - name: github
      type: github-app
      config:
        appId: "12345"
        installationId: "67890"
        privateKeyPath: /etc/secrets/github-app.pem

    - name: slack
      type: static-token
      config:
        token: xoxb-slack-bot-token

  webhookSecrets:
    - source: github
      secret: whsec_abc123

listeners:
  - name: github-webhooks
    type: webhook
    config:
      path: /webhooks/github

reactors:
  - name: pr-comment
    match: >
      payload.action == "opened" &&
      has(payload.pull_request)
    provider: http
    auth: github
    inputs:
      method: POST
      url:
        template: >-
          https://api.github.com/repos/{{ .payload.repository.full_name }}/issues/{{ .payload.number }}/comments
      body:
        template: '{"body": "Thanks for the PR!"}'

  - name: error-alert
    match: >
      has(payload.severity) &&
      payload.severity == "ERROR"
    provider: http
    auth: slack
    inputs:
      method: POST
      url: https://slack.com/api/chat.postMessage
      body:
        template: '{"channel": "#alerts", "text": "Error: {{ .payload.message }}"}'
```

## Input Resolution

Reactor inputs support multiple resolution strategies. When a value is a simple scalar, it's used as-is. For dynamic values, use a map with one of these keys:

| Key | Description | Example |
|-----|-------------|---------|
| `template` | Go template rendered against event data | `"PR #{{ .payload.number }}"` |
| `expr` | CEL expression evaluated against event | `'payload.repository.full_name'` |
| `payloadValue.propertyPaths` | CEL paths tried in order, first match wins | `["payload.number", "payload.issue.number"]` |
| `fromEnv` | Environment variable name | `"SLACK_TOKEN"` |
| `fromFile` | File path to read | `"/etc/config/template.txt"` |
| `valueFrom.secretKeyRef` | GCP Secret Manager reference | see below |

**Resolution priority** (first match wins):
1. `valueFrom.secretKeyRef`
2. `payloadValue.propertyPaths`
3. `fromFile`
4. `fromEnv`
5. `expr`
6. `template`
7. static scalar

### Template Functions

Available in `template` inputs:

| Function | Description |
|----------|-------------|
| `upper` | Uppercase string |
| `lower` | Lowercase string |
| `trimSpace` | Trim whitespace |
| `contains` | Check substring |
| `hasPrefix` | Check prefix |
| `hasSuffix` | Check suffix |
| `replace` | String replacement |
| `split` | Split by delimiter |
| `join` | Join with delimiter |
| `trimPrefix` | Remove prefix |
| `trimSuffix` | Remove suffix |
| `default` | Default value if empty |
| `toJSON` | Marshal to JSON string |

## Listeners

Listeners bring events into the reactor. Each entry has a `name`, a `type`, and a
type-specific `config` block:

```yaml
listeners:
  - name: <name>
    type: <type>
    config:
      # type-specific settings
```

| Type | Delivery | Notes |
|------|----------|-------|
| `webhook` | HTTP push | Served by the built-in HTTP server on `/webhook/:source`. No background runtime. |
| `cloudevents` | HTTP push | Served by the HTTP server on `/cloudevents`. No background runtime. |
| `http` | HTTP push | Generic push served on `/events`. No background runtime. |
| `pubsub` | Pull | Background listener that pulls from a GCP Pub/Sub subscription. |

HTTP-served types (`webhook`, `cloudevents`, `http`) are handled by the API server
routes, so declaring them is optional and starts no extra goroutine. The `pubsub`
type starts a background listener that runs alongside the HTTP server; an unknown
type fails startup.

### Pub/Sub Pull Listener

The `pubsub` listener pulls messages from an existing subscription using
Application Default Credentials (ADC). It converts each message into the same
event shape as HTTP push (attributes map to `ce-*` CloudEvent fields; the message
body is decoded as the event payload), dispatches it through the matcher and
reactors, then acks. Messages that fail conversion or whose handler panics are
nacked so Pub/Sub can redeliver or route them to a dead-letter topic.

```yaml
listeners:
  - name: platform-assets
    type: pubsub
    config:
      projectId: my-gcp-project            # required
      subscriptionId: platform-assets-sub  # required
      maxOutstandingMessages: 100          # optional, flow-control cap
      numGoroutines: 2                     # optional, concurrent pull streams
```

| Field | Required | Description |
|-------|----------|-------------|
| `projectId` | yes | GCP project that owns the subscription. |
| `subscriptionId` | yes | Existing Pub/Sub subscription ID to pull from. |
| `maxOutstandingMessages` | no | Max unacked messages held locally (flow control). |
| `numGoroutines` | no | Number of concurrent streaming-pull goroutines. |

The reactor identity needs `roles/pubsub.subscriber` on the subscription. Ack
deadlines, retry policy, and dead-lettering are properties of the subscription
itself and are configured where the subscription is provisioned, not here.

## Hot-Reload

The config file is watched via fsnotify. Changes are automatically applied without restarting the server. Invalid configs are rejected with an error log (the old config remains active).
