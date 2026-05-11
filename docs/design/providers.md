---
title: "Providers"
weight: 3
---

# Providers

Providers are the execution backends for reactors. Each provider implements a single interface:

```go
type Provider interface {
    Name() string
    Execute(ctx context.Context, inputs map[string]any, event message.Event) (*Result, error)
}
```

## Built-in Providers

### echo

Returns inputs as output. Used for testing and debugging.

```yaml
provider: echo
inputs:
  msg: "Hello from event-reactor"
  event_id:
    expr: 'id'
```

### http

Sends an HTTP request. Supports auth token injection.

```yaml
provider: http
auth: my-handler
inputs:
  method: POST
  url: https://api.example.com/notify
  contentType: application/json
  body:
    template: '{"text": "Event {{ .id }} from {{ .source }}"}'
  headers:
    expr: '{"X-Custom": "value"}'
```

**Inputs:**
| Name | Required | Description |
|------|----------|-------------|
| `url` | yes | Target URL |
| `method` | no | HTTP method (default: POST) |
| `body` | no | Request body (string or object) |
| `contentType` | no | Content-Type header (default: application/json) |
| `headers` | no | Additional headers (map) |

### exec

Runs a local command.

```yaml
provider: exec
inputs:
  command: /usr/local/bin/notify.sh
  args:
    - "--event-id"
    - template: "{{ .id }}"
  dir: /tmp
  stdin:
    template: "{{ toJSON .payload }}"
```

**Inputs:**
| Name | Required | Description |
|------|----------|-------------|
| `command` | yes | Command to execute |
| `args` | no | Command arguments (list) |
| `dir` | no | Working directory |
| `stdin` | no | Standard input content |

### log

Logs to the server's structured logger.

```yaml
provider: log
inputs:
  level: warn
  message:
    template: "Event {{ .id }}: {{ .payload.action }}"
```

**Inputs:**
| Name | Required | Description |
|------|----------|-------------|
| `level` | no | Log level: debug, info, warn, error (default: info) |
| `message` | no | Log message (default: event summary) |

## Creating a Provider

Implement the `Provider` interface and register it:

```go
type MyProvider struct{}

func (p *MyProvider) Name() string { return "my-provider" }

func (p *MyProvider) Execute(ctx context.Context, inputs map[string]any, event message.Event) (*reactor.Result, error) {
    // Your logic here
    return &reactor.Result{Provider: "my-provider", Output: result}, nil
}

// Register in providers.go or cmd wiring:
reg.Register(&MyProvider{})
```
