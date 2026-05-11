# event-reactor

Event-driven automation engine. Listens for events, matches them with CEL expressions, and dispatches reactions via pluggable providers.

## Features

- **Event Sources**: HTTP push (`/events`, `/cloudevents`, `/webhook/:source`), GCP Pub/Sub (planned)
- **Matching**: [CEL](https://github.com/google/cel-go) expressions with compiled caching
- **Providers**: echo, http, exec, log (extensible via the Provider interface)
- **Templating**: Go `text/template` with custom functions for input resolution
- **Auth**: Named auth handlers (static tokens, GitHub App, GitHub PAT, OAuth2 client credentials, GCP service account) injected into outbound calls
- **Webhook Validation**: HMAC-SHA256 signature verification on inbound webhooks
- **Config Hot-Reload**: File watcher reloads config on changes (fsnotify)
- **Reliability**: Token-bucket rate limiter, circuit breaker
- **MCP Server**: Model Context Protocol over stdio for AI-assisted configuration
- **CLI**: `er test match|template|config|reactor` for local development/testing

## Quick Start

```bash
# Build
task build

# Run server
dist/er run server --config config.yaml

# Test a CEL expression against an event
dist/er test match 'payload.action == "opened"' -e event.json

# Render a template
dist/er test template -t 'PR #{{ .payload.number }}' -e event.json

# Validate config
dist/er test config config.yaml

# Dry-run a reactor
dist/er test reactor -c config.yaml -e event.json -n my-reactor --dry-run

# Start MCP server
dist/er mcp
```

## Configuration

```yaml
apiVersion: event-reactor.io/v1
kind: ServerConfig

server:
  port: 8080
  metricsPort: 9090

observability:
  logging:
    level: info
    format: json

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

    - name: gcp
      type: service-account
      defaultScopes:
        - https://www.googleapis.com/auth/cloud-platform
      config:
        keyFile: /etc/secrets/sa-key.json

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

  - name: error-to-slack
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

### Auth Handler Types

| Type | Description | Required Config |
|------|-------------|-----------------|
| `static-token` | Fixed bearer token | `token`, optional `tokenType` |
| `github-token` | GitHub PAT | `token` |
| `github-app` | GitHub App installation token | `appId`, `installationId`, `privateKeyPath` |
| `oauth2-client-credentials` | OAuth2 client credentials flow | `tokenUrl`, `clientId`, `clientSecret` |
| `service-account` | GCP service account | `keyFile` |

Reactors reference auth handlers by name via the `auth` field. The resolved token is injected as the `Authorization` header on outbound HTTP calls.

### Input Resolution

Reactor inputs support multiple resolution strategies (highest priority first):

| Source | Config | Description |
|--------|--------|-------------|
| Secret | `valueFrom.secretKeyRef` | GCP Secret Manager |
| Payload | `payloadValue.propertyPaths` | CEL path into event payload |
| File | `fromFile` | Read from local file |
| Env | `fromEnv` | Environment variable |
| Expr | `expr` | CEL expression |
| Template | `template` | Go template with event data |
| Static | scalar value | Literal value |

## Architecture

```
Event Source --> Listener --> Adapter --> Matcher (CEL) --> Reactor
                                                            |
                                                   Auth + Input Resolution
                                                            |
                                                         Provider
                                                     (echo/http/exec/log)
```

## Development

```bash
task build          # Build binary to dist/
task test           # Run tests
task lint           # golangci-lint
task vet            # go vet
task coverage       # Coverage report
```

## Deployment

Kustomize manifests in `deploy/`:

```bash
kubectl apply -k deploy/base           # Minimal deployment
kubectl apply -k deploy/production     # HPA + PDB + resource tuning
```

Container images published to `ghcr.io/oakwood-commons/event-reactor` on tag push.

## License

Apache License 2.0