# event-reactor -- Project Plan

> Open-source, event-driven automation engine for Kubernetes and GCP.
> Receive events, filter with CEL, react with pluggable handlers.

**Repository**: [oakwood-commons/event-reactor](https://github.com/oakwood-commons/event-reactor)

---

## Vision

A complete rewrite of event-reactor as a single, lightweight Go binary that
subscribes to multiple event sources (GCP Pub/Sub, CloudEvents, HTTP webhooks),
evaluates each event against CEL filter expressions, and dispatches matched
events to **scafctl providers** as reactors -- all defined in a declarative
YAML configuration.

The [v1 codebase](https://github.com/kcloutie/event-reactor) serves as a
reference for the event pipeline concept (listener → matcher → reactor), but
the new version is built from scratch on top of the scafctl provider SDK.
No backwards compatibility with v1 is maintained.

Key design decisions:
- **Reactors are scafctl providers** -- no hand-rolled integrations
- **Auth is scafctl auth handlers** -- GCP, GitHub, Entra ID out of the box
- **Plugins are scafctl plugins** -- same gRPC protocol, same binaries
- **Listeners are event-reactor's contribution** -- the one thing scafctl can't do
- **CLI testing** -- `er test` exercises providers, CEL, and templates locally

One instance handles many event types. Configuration hot-reloads without
restarts. Deploys to Kubernetes or Cloud Run with zero platform-specific
code changes.

---

## Competitive Landscape

| Tool | Stars | What it does | How event-reactor differs |
|------|-------|-------------|--------------------------|
| **Knative Eventing** | 1.5k | Full event-mesh platform for K8s. Brokers, channels, subscriptions, sources, sinks. CloudEvents native. | Heavy -- requires Knative Serving, Istio/Kourier. event-reactor is a single binary with no cluster-level CRDs. |
| **Argo Events** | 2.7k | Event sources + sensors + triggers for Kubernetes. 20+ sources, 10+ triggers. CNCF project. | Requires Argo Workflows as a dependency for most interesting triggers. K8s-only (CRD-based). event-reactor runs anywhere (K8s, Cloud Run, local). |
| **TriggerMesh** | 609 | CloudEvents integration platform. Sources, targets, transformations as K8s CRDs. | **Archived** (Nov 2024). Company folded. event-reactor fills this gap. |
| **Falcosidekick** | 665 | Fan-out Falco security events to 60+ outputs. | Falco-specific input only. No CEL filtering. No plugin system. event-reactor is source-agnostic. |
| **Botkube** | 2.3k | K8s monitoring bot for Slack/Discord/Teams. Plugin system for executors and sources. | ChatOps-focused, not general event automation. |
| **Nuclio** | 5.7k | High-performance serverless FaaS platform. Multi-language runtimes. | Full FaaS -- heavy, complex. event-reactor is config-driven, no user code required for simple reactions. |

### Why event-reactor is worth creating

1. **TriggerMesh is dead** -- the most similar tool was archived. There is a gap
   for a lightweight, config-driven event router that is not tied to Knative.
2. **No existing tool combines**: CEL filtering + Go template transformation +
   pluggable reactors + plugin system + runs outside Kubernetes.
3. **Argo Events and Knative are K8s-only** -- they require CRDs, controllers,
   and cluster-level permissions. event-reactor is a single binary.
4. **Falcosidekick proves the pattern works** (665 stars, 60+ outputs) but is
   locked to Falco. event-reactor generalizes this for any event source.
5. **The plugin system** (from scafctl) makes it genuinely extensible without
   bloating the core binary.
6. **Cloud Run and local dev** are first-class targets, not afterthoughts.

---

## Architecture

```
                    ┌───────────────────────────────────────────────────────┐
                    │                  event-reactor                        │
                    │                                                       │
 ┌────────────┐    │  ┌───────────┐    ┌──────────┐                        │
 │ GCP Pub/Sub│───▶│  │ Listener  │───▶│ Adapter  │                        │
 └────────────┘    │  └───────────┘    └────┬─────┘                        │
                   │                        │                               │
 ┌────────────┐    │  ┌───────────┐         ▼                               │
 │CloudEvents │───▶│  │ Listener  │    ┌──────────┐                        │
 │ (K8s/HTTP) │    │  └───────────┘    │ Matcher  │ CEL                    │
 └────────────┘    │                   │ Engine   │                        │
                   │  ┌───────────┐    └────┬─────┘                        │
 ┌────────────┐    │  │ Listener  │         │                               │
 │  Webhook   │───▶│  └───────────┘         ▼                               │
 │  (push)    │    │                   ┌──────────┐                        │
 └────────────┘    │  ┌───────────┐    │ Reactor  │                        │
                   │  │ Plugin    │    │ Dispatch │                        │
 ┌────────────┐    │  │ Listener  │    └────┬─────┘                        │
 │  Plugin    │───▶│  │ (gRPC)    │         │                               │
 │ (gRPC)     │    │  └───────────┘         ▼                               │
 └────────────┘    │           ┌─────────────────────────┐                 │
                   │           │  scafctl Provider SDK    │                 │
                   │           │                         │                 │
                   │           │  ┌─────────┐ ┌────────┐ │  ┌───────────┐  │
                   │           │  │Built-in │ │Plugin  │ │  │ scafctl   │  │
                   │           │  │Providers│ │Provid. │ │  │ Auth      │  │
                   │           │  │(http,   │ │(github,│ │  │ Handlers  │  │
                   │           │  │ cel,    │ │ exec,  │ │  │(gcp,entra,│  │
                   │           │  │ static, │ │ env,   │ │  │ github)   │  │
                   │           │  │ file)   │ │ etc.)  │ │  └───────────┘  │
                   │           │  └─────────┘ └────────┘ │                 │
                   │           └─────────────────────────┘                 │
                   │                                                       │
                   │  ┌─────────────────────────────────────────────────┐  │
                   │  │  Observability (OTEL)                            │  │
                   │  │  Traces · Metrics · Structured Logs              │  │
                   │  └─────────────────────────────────────────────────┘  │
                   └───────────────────────────────────────────────────────┘
```

### Core Pipeline

```
Event Source  ──▶  Listener  ──▶  Adapter  ──▶  Matcher (CEL)  ──▶  Reactor (scafctl Provider)
```

1. **Listener** -- connects to an event source and produces normalized event envelopes (event-reactor only)
2. **Adapter** -- bridges listeners to the matching/reaction pipeline (event-reactor only)
3. **Matcher** -- evaluates CEL expressions against event attributes and payload (shared CEL with scafctl)
4. **Reactor** -- executes a **scafctl provider** with the event data as input (scafctl SDK)
5. **Auth** -- scafctl auth handlers manage credentials for GitHub, GCP, Entra ID, etc. (scafctl SDK)

### What scafctl provides vs. what event-reactor owns

| Concern | Owner | Why |
|---------|-------|-----|
| **Listening for events** | event-reactor | scafctl has no listener concept |
| **Event matching (CEL)** | event-reactor | Filtering events against config. Shares CEL evaluation from scafctl |
| **Reaction execution** | **scafctl providers** | `http`, `github`, `exec`, `env`, `file`, `secret`, etc. are already built |
| **Authentication** | **scafctl auth handlers** | GitHub tokens, GCP credentials, Entra ID -- already solved |
| **Plugin protocol** | **scafctl plugin SDK** | Same gRPC protocol, same plugin binaries work in both tools |
| **Template rendering** | Shared | Both use Go templates with custom funcs |
| **Server / API** | event-reactor | HTTP server, health, config loading |
| **CLI testing** | event-reactor | `er test` commands that exercise providers, matchers, and templates |

### Why build on scafctl?

- **20+ providers for free** (http, github, exec, env, file, directory,
  git, hcl, identity, metadata, secret, sleep, cel, static, go-template, etc.)
- **Auth handlers for free** (GCP, Entra/Azure AD, GitHub App, PAT)
- **Shared plugin ecosystem** -- any scafctl plugin provider works as an
  event-reactor reactor with zero changes
- **Single maintenance burden** -- bug fixes and new providers in scafctl
  automatically benefit event-reactor
- **Small binary** -- niche providers are plugins, not compiled in

---

## Event Sources

### Tier 1 -- Ship at launch

| Source | Protocol | Description |
|--------|----------|-------------|
| **GCP Pub/Sub** | Pull subscription | Primary GCP eventing. Supports ordering keys, dead-letter topics, exactly-once delivery. |
| **CloudEvents** | HTTP (push) | CNCF standard. Native to Knative, Kubernetes EventBridge, and many cloud services. Receive via HTTP sink. |
| **HTTP Webhook** | HTTP (push) | Generic push endpoint for any system that can POST JSON (GitHub, GitLab, Stripe, etc.). |

### Tier 2 -- Future

| Source | Protocol | Description |
|--------|----------|-------------|
| Kafka | Pull | For organizations using Kafka-based event buses. |
| NATS | Pull/Push | Lightweight cloud-native messaging. |
| AWS EventBridge | HTTP | Cross-cloud eventing. |

---

## Reactors (powered by scafctl Providers)

Reactors are **scafctl providers** invoked with event data as input.
event-reactor imports the scafctl provider SDK and registry, giving it
access to all built-in and plugin providers without reimplementation.

### How it works

1. Config says `provider: http` (or `provider: github`, etc.)
2. event-reactor resolves the provider from scafctl's registry
3. Event attributes and payload are injected as template/CEL variables
4. Provider inputs are resolved (static, template, CEL expression, secret)
5. `provider.Execute(ctx, inputs)` is called
6. Output is logged, metrics recorded, errors retried

### Available providers (via scafctl)

**Built-in** (compiled into er binary):

| Provider | Description |
|----------|-------------|
| `http` | HTTP requests to any URL. Supports all methods, headers, auth, retries. |
| `cel` | Evaluate CEL expressions. For computed inputs and transformations. |
| `static` | Return a static value. For constants and defaults. |
| `file` | Read/write files. For template rendering and config generation. |
| `go-template` | Render Go templates. For message bodies, comments, etc. |
| `message` | Structured message formatting. |
| `validation` | JSON Schema validation. For input checking. |
| `debug` | Inspect data at runtime. For troubleshooting. |

**Plugin providers** (auto-resolved from OCI catalog):

| Provider | Description |
|----------|-------------|
| `github` | Full GitHub API: issues, PRs, comments, releases, branches, rulesets, security. |
| `exec` | Execute shell commands (bash, PowerShell, etc.). |
| `env` | Read environment variables. |
| `directory` | List and read directory contents. |
| `git` | Git operations (clone, diff, log). |
| `hcl` | Parse and generate HCL/Terraform files. |
| `identity` | Auth claims and identity info. |
| `metadata` | Runtime metadata. |
| `secret` | Encrypted secret management. |
| `sleep` | Delays (for rate limiting, sequencing). |
| `goscript` | Run Go scripts inline. |

### Missing providers -- contribute upstream

If scafctl is missing a provider that event-reactor needs, create it as a
scafctl plugin provider. This benefits both projects:

- **Needed for event-reactor**: email/SMTP, Webex messaging, GCP Pub/Sub
  publish, GCP Secret Manager write, GCP BigQuery insert, GCP Storage write,
  PagerDuty, Slack, Jira
- **Process**: Build as a scafctl plugin → test with `er test provider` →
  publish to OCI → submit PR upstream to scafctl plugin catalog

### Reactor input resolution

Reactor inputs are resolved before being passed to the scafctl provider.
Resolution supports all scafctl value reference types plus event-specific
sources:

```yaml
reactors:
  - name: notify-on-error
    match: 'payload.severity == "ERROR"'
    provider: http
    inputs:
      # Static value
      method: POST
      url: https://hooks.slack.com/services/T00/B00/xxx

      # Go template with event data
      body:
        template: |
          {"text": "Error in {{ .payload.resource.labels.pod_name }}: {{ .payload.textPayload }}"}

      # CEL expression
      headers:
        expr: '{"Content-Type": "application/json"}'

      # From secret (via scafctl auth handler or secret provider)
      authorization:
        fromSecret: slack-webhook-token

      # From event attribute
      traceId:
        fromAttribute: attributes.traceId
```

---

## Configuration

A single YAML file defines everything one instance handles.
The server watches the config file and hot-reloads on changes.

```yaml
apiVersion: event-reactor.io/v1
kind: ServerConfig

server:
  port: 8080
  metricsPort: 9090
  healthCheck:
    liveness: /health/live
    readiness: /health/ready

# --- Observability ---
observability:
  logging:
    level: info                  # debug, info, warn, error
    format: json                 # json, text
    backend: otlphttp            # stdout, otlphttp, both
    otel:
      endpoint: "http://localhost:4318"
  tracing:
    enabled: true
    exporter: otlp
    endpoint: "localhost:4317"
    sampleRate: 0.1
  metrics:
    enabled: true
    prometheus: true             # expose /metrics endpoint

# --- Authentication (scafctl auth handlers) ---
auth:
  handlers:
    - name: gcp
      type: gcp                  # uses ADC / WIF automatically
    - name: github
      type: github-app
      config:
        appId: 12345
        installationId: 67890
        privateKeySecret: github-app-key

# --- Listeners (event-reactor only) ---
listeners:
  - name: gcp-pubsub-events
    type: pubsub
    config:
      projectId: my-project
      subscriptionId: event-reactor-sub

  - name: cloudevents-sink
    type: cloudevents
    config:
      path: /events

  - name: github-webhooks
    type: webhook
    config:
      path: /webhooks/github
      secretRef:
        name: github-webhook-secret
        projectId: my-project

# --- Reactors (scafctl providers) ---
reactors:
  # Example: post a GitHub comment on PR events
  - name: pr-review-checklist
    match: >
      payload.action == "opened" &&
      payload.pull_request.base.ref == "main"
    provider: github
    inputs:
      operation: create-comment
      owner:
        template: "{{ .payload.repository.owner.login }}"
      repo:
        template: "{{ .payload.repository.name }}"
      number:
        template: "{{ .payload.pull_request.number }}"
      body:
        template: |
          ## Review Checklist
          - [ ] Tests pass
          - [ ] Docs updated
          - [ ] No secrets in code
    auth: github

  # Example: forward errors to a webhook (Slack, etc.)
  - name: error-to-slack
    match: >
      has(payload.severity) &&
      payload.severity in ["ERROR", "CRITICAL"]
    provider: http
    inputs:
      method: POST
      url: https://hooks.slack.com/services/T00/B00/xxx
      headers:
        expr: '{"Content-Type": "application/json"}'
      body:
        template: |
          {
            "text": ":rotating_light: *{{ .payload.severity }}* in `{{ .payload.resource.labels.pod_name }}`\n{{ .payload.textPayload }}"
          }

  # Example: execute a shell command
  - name: run-rotation-script
    match: 'attributes.eventType == "SECRET_VERSION_EXPIRING"'
    provider: exec
    inputs:
      command: /usr/local/bin/rotate-secret.sh
      args:
        template: '["{{ .attributes.secretName }}", "{{ .attributes.projectId }}"]'
    retry:
      maxAttempts: 3
      backoff: exponential
```

---

## Use Cases

### 1. GCP Secret Rotation

```
GCP Secret Manager (eventarc)
    │  SECRET_VERSION_EXPIRING event
    ▼
Pub/Sub topic ──▶ event-reactor
    │
    │  CEL: attributes.eventType == "SECRET_VERSION_EXPIRING"
    ▼
exec provider (rotate-secret.sh)
    │  Generate new secret, update SM
    ▼
http provider (Slack webhook)
    │  Notify team of rotation
    ▼
Done
```

### 2. Log Filtering and Forwarding

```
GCP Cloud Logging (log sink)
    │  Structured log entries
    ▼
Pub/Sub topic ──▶ event-reactor
    │
    │  CEL: payload.severity in ["ERROR", "CRITICAL"]
    ▼
http provider (POST to Loki/Splunk/Datadog)
    │  Forward filtered logs
    ▼
Done
```

### 3. GitHub PR Automation

```
GitHub webhook (push)
    │  pull_request events
    ▼
event-reactor (HTTP webhook listener)
    │
    │  CEL: payload.action == "opened" &&
    │       payload.pull_request.base.ref == "main"
    ▼
github provider (create-comment)
    │  Post review checklist
    ▼
http provider (Webex/Slack webhook)
    │  Notify team
    ▼
Done
```

### 4. Multi-Event Instance

A single instance handles all of the above simultaneously:

```yaml
listeners:
  - name: secret-events
    type: pubsub
    config: { projectId: my-project, subscriptionId: secret-sub }

  - name: log-events
    type: pubsub
    config: { projectId: my-project, subscriptionId: k8s-logs-sub }

  - name: github-hooks
    type: webhook
    config: { path: /webhooks/github }

reactors:
  - name: rotate-secrets
    match: "attributes.eventType == 'SECRET_VERSION_EXPIRING'"
    provider: exec
    inputs:
      command: /usr/local/bin/rotate-secret.sh
      args:
        template: '["{{ .attributes.secretName }}"]'

  - name: forward-error-logs
    match: "payload.severity in ['ERROR', 'CRITICAL']"
    provider: http
    inputs:
      method: POST
      url: https://loki.internal/loki/api/v1/push
      body:
        template: '{{ toJson .payload }}'

  - name: pr-comments
    match: "payload.action == 'opened'"
    provider: github
    inputs:
      operation: create-comment
      owner:
        template: "{{ .payload.repository.owner.login }}"
      repo:
        template: "{{ .payload.repository.name }}"
      number:
        template: "{{ .payload.pull_request.number }}"
      body: "## Review checklist\n- [ ] Tests pass"
    auth: github
```

---

## Deployment

### Kubernetes

Kustomize-based manifests with per-environment overlays:

```
k8s/
├── base/
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── serviceaccount.yaml
│   ├── hpa.yaml                  # Horizontal Pod Autoscaler
│   ├── pdb.yaml                  # Pod Disruption Budget
│   ├── otel-sidecar.yaml         # OTEL Collector sidecar
│   └── kustomization.yaml
└── overlays/
    ├── dev/
    └── prod/
```

**Key deployment patterns**:

- **OTEL Collector sidecar** -- app sends traces/metrics/logs to `localhost:4318`,
  sidecar exports to Google Cloud Monitoring, Dynatrace, or any OTLP backend
- **WIF (Workload Identity Federation)** -- no service account keys; Kubernetes
  ServiceAccount federated to GCP IAM via OIDC token exchange
- **Security hardening** -- `runAsNonRoot`, `readOnlyRootFilesystem`,
  `drop: [ALL]` capabilities, `RuntimeDefault` seccomp
- **Topology spread** -- schedule across zones for HA
- **Health probes** -- `/health/live` and `/health/ready` endpoints
- **Graceful shutdown** -- drain in-flight events before termination

### Cloud Run

event-reactor is stateless by design and Cloud Run-ready:

- No sidecars needed -- OTEL logs/traces export directly to Google Cloud
- Pub/Sub push subscriptions deliver to the Cloud Run service URL
- CloudEvents and webhooks work natively over HTTP
- Min instances = 1 recommended (Pub/Sub pull requires a running process)
- For Pub/Sub pull mode, use Cloud Run **always-on CPU** allocation

```bash
gcloud run deploy event-reactor \
  --image ghcr.io/oakwood-commons/event-reactor:latest \
  --args="run,server,-c,/config/server-config.yaml" \
  --set-env-vars="GOOGLE_APPLICATION_CREDENTIALS=/secrets/cred-config.json" \
  --cpu-boost \
  --min-instances=1 \
  --port=8080
```

### Local Development

```bash
# 1. Build
task build

# 2. Run with a local config
./er run server -c config/local.yaml

# 3. Test a CEL expression against a sample event
er test match --expression 'attributes.eventType == "test"' \
  --event testdata/sample-event.json

# 4. Test a provider execution
er test provider --provider http \
  --inputs '{"url": "https://httpbin.org/post", "method": "POST", "body": "hello"}'

# 5. Test a reactor config against a sample event (dry-run)
er test reactor --config config/local.yaml \
  --reactor pr-review-checklist \
  --event testdata/github-pr-opened.json \
  --dry-run

# 6. Send a test event to the running server
curl -X POST http://localhost:8080/events \
  -H "Content-Type: application/json" \
  -d '{"attributes": {"eventType": "test"}, "payload": {"message": "hello"}}'

# 7. Run all tests
task test

# 8. Optional: local OTEL collector
docker run -p 4318:4318 otel/opentelemetry-collector-contrib
export ER_LOGGING_BACKEND=otlphttp
export ER_LOGGING_OTEL_ENDPOINT="http://localhost:4318"
./er run server -c config/local.yaml
```

---

## Observability

Adopt the OTEL pattern from jqapi:

### Structured Logging

- JSON structured logs by default
- Configurable backend: `stdout`, `otlphttp`, or `both`
- Automatic trace context injection (trace ID, span ID in log entries)
- Fallback to stdout if OTLP endpoint is unavailable

### Metrics (Prometheus)

Exposed at `/metrics`:

| Metric | Type | Description |
|--------|------|-------------|
| `er_events_received_total` | counter | Events received per listener/source |
| `er_events_matched_total` | counter | Events that matched a CEL filter |
| `er_events_dropped_total` | counter | Events that matched no filter |
| `er_reactor_executions_total` | counter | Reactor executions per type/status |
| `er_reactor_duration_seconds` | histogram | Reactor execution latency |
| `er_cel_evaluation_duration_seconds` | histogram | CEL expression evaluation time |
| `er_pubsub_ack_latency_seconds` | histogram | Time from receive to ack/nack |

### Distributed Tracing

- Every event gets a trace from receive through reactor execution
- Propagate CloudEvents `traceparent` header when present
- Configurable sample rate (default 10%)
- Export via OTLP (gRPC or HTTP)

---

## CLI Testing (`er test`)

Test scafctl providers, CEL expressions, templates, and full reactor pipelines
from the command line -- no running server or real events needed.

### `er test match` -- test CEL expressions

```bash
# Test an expression against a sample event file
er test match \
  --expression 'attributes.eventType == "SECRET_VERSION_EXPIRING"' \
  --event testdata/secret-expiring.json

# Output:
# MATCH: expression evaluated to true
# Evaluation time: 42us

# Test against inline JSON
er test match \
  --expression 'payload.severity in ["ERROR", "CRITICAL"]' \
  --event '{"attributes": {}, "payload": {"severity": "ERROR", "message": "disk full"}}'

# Test multiple expressions (all must match)
er test match \
  --expression 'has(payload.severity)' \
  --expression 'payload.severity == "ERROR"' \
  --event testdata/log-entry.json
```

### `er test provider` -- test a scafctl provider

```bash
# Test the http provider
er test provider --provider http \
  --inputs '{"url": "https://httpbin.org/post", "method": "POST", "body": "hello"}'

# Output:
# Provider: http
# Status:   success
# Output:   {"statusCode": 200, "body": {...}, "headers": {...}}

# Test the github provider (uses configured auth)
er test provider --provider github \
  --inputs '{"operation": "get-pull-request", "owner": "oakwood-commons", "repo": "event-reactor", "number": 1}'

# Test with auth handler
er test provider --provider github --auth github \
  --inputs '{"operation": "list-comments", "owner": "oakwood-commons", "repo": "event-reactor", "number": 1}'

# Test the exec provider
er test provider --provider exec \
  --inputs '{"command": "echo", "args": ["hello", "world"]}'

# List all available providers
er test provider --list
```

### `er test template` -- test Go template rendering

```bash
# Render a template against event data
er test template \
  --template 'Error in {{ .payload.resource.labels.pod_name }}: {{ .payload.textPayload }}' \
  --event testdata/log-entry.json

# Output:
# Error in my-app-pod-abc123: connection timeout to database

# Render from a template file
er test template \
  --template-file templates/slack-notification.tpl \
  --event testdata/log-entry.json
```

### `er test reactor` -- test a full reactor pipeline

```bash
# Test a reactor from config against a sample event (dry-run, no side effects)
er test reactor \
  --config config/server.yaml \
  --reactor pr-review-checklist \
  --event testdata/github-pr-opened.json \
  --dry-run

# Output:
# Reactor:    pr-review-checklist
# Match:      true (expression: payload.action == "opened" && ...)
# Provider:   github
# Operation:  create-comment
# Resolved inputs:
#   owner: "oakwood-commons"
#   repo:  "event-reactor"
#   number: 42
#   body:  "## Review Checklist\n- [ ] Tests pass\n..."
# Mode:       dry-run (no execution)

# Actually execute (careful!)
er test reactor \
  --config config/server.yaml \
  --reactor pr-review-checklist \
  --event testdata/github-pr-opened.json

# Test all reactors in a config against an event (shows which match)
er test reactor \
  --config config/server.yaml \
  --event testdata/github-pr-opened.json \
  --dry-run --all
```

### `er test config` -- validate a config file

```bash
# Validate config syntax, provider availability, and CEL expressions
er test config --config config/server.yaml

# Output:
# Config:     config/server.yaml
# Listeners:  3 (pubsub, cloudevents, webhook)
# Reactors:   5
#   pr-review-checklist  provider=github    expression=valid
#   error-to-slack       provider=http      expression=valid
#   rotate-secrets       provider=exec      expression=valid
#   forward-logs         provider=http      expression=valid
#   audit-trail          provider=http      expression=valid
# Auth:       2 handlers (gcp, github)
# Status:     OK
```

---

## MCP Server (`er mcp`)

event-reactor includes an MCP (Model Context Protocol) server so AI assistants
can help users configure listeners, write CEL filters, build reactor configs,
and troubleshoot -- the same pattern as scafctl's MCP server.

### Why MCP?

Writing event-reactor configs requires knowing: what listeners are available,
what providers exist, what inputs each provider expects, how CEL expressions
work against event shapes, and how Go templates render. An AI assistant with
MCP access can answer all of these interactively instead of the user reading
docs.

### MCP Tools

| Tool | Description |
|------|-------------|
| `list_providers` | List all available providers (built-in + plugins) with descriptions |
| `get_provider_schema` | Get the input schema for a specific provider (JSON Schema) |
| `run_provider` | Execute a provider with inputs and return the output |
| `list_listeners` | List available listener types with configuration schemas |
| `test_cel_expression` | Evaluate a CEL expression against a sample event, return true/false + errors |
| `render_template` | Render a Go template against event data, return the output |
| `test_reactor` | Dry-run a reactor config against a sample event (match + resolve inputs) |
| `validate_config` | Validate a server config file (syntax, providers, CEL expressions) |
| `list_auth_handlers` | List configured auth handlers and their status |
| `get_event_schema` | Describe the event envelope shape (attributes, payload) for a listener type |

### Example AI Workflow

```
User: "I want to post a Slack message when a GCP secret is about to expire"

AI (calls list_listeners):
  → pubsub, cloudevents, webhook

AI (calls list_providers):
  → http, github, exec, cel, static, ...

AI (calls get_provider_schema for http):
  → { url: string, method: string, headers: map, body: string, ... }

AI (calls test_cel_expression):
  → expression: attributes.eventType == "SECRET_VERSION_EXPIRING"
  → event: {"attributes": {"eventType": "SECRET_VERSION_EXPIRING"}, ...}
  → result: true

AI generates config:
  listeners:
    - name: secret-events
      type: pubsub
      config: { projectId: my-project, subscriptionId: secret-sub }
  reactors:
    - name: slack-notify
      match: 'attributes.eventType == "SECRET_VERSION_EXPIRING"'
      provider: http
      inputs:
        method: POST
        url: https://hooks.slack.com/services/...
        body:
          template: '{"text": "Secret {{ .attributes.secretName }} expiring"}'

AI (calls validate_config):
  → Status: OK

AI (calls test_reactor with dry-run):
  → Match: true, Provider: http, Resolved inputs: {...}
```

### Implementation

The MCP server reuses the same code paths as `er test` CLI commands.
Both the CLI and MCP server call the same internal functions:

```
er test match    ──▶  pkg/test/match.go    ◀──  MCP test_cel_expression
er test provider ──▶  pkg/test/provider.go ◀──  MCP run_provider
er test template ──▶  pkg/test/template.go ◀──  MCP render_template
er test reactor  ──▶  pkg/test/reactor.go  ◀──  MCP test_reactor
er test config   ──▶  pkg/test/config.go   ◀──  MCP validate_config
```

MCP transport: stdio (for editor integration) and HTTP/SSE (for remote use).
Uses the same MCP SDK as scafctl.

---

## CEL Integration

CEL expressions power event filtering. Use the same CEL library as
[scafctl](https://github.com/oakwood-commons/scafctl) for consistency.

```cel
// Match by event attribute
attributes.eventType == "SECRET_VERSION_EXPIRING"

// Match by payload field with existence check
has(payload.severity) && payload.severity in ["ERROR", "CRITICAL"]

// Complex filtering with string functions
attributes.source.startsWith("projects/my-project") &&
  payload.resource.type == "k8s_container" &&
  !payload.textPayload.contains("health check")

// Regex matching
payload.message.matches("^ERROR:.*timeout.*$")

// Numeric comparisons
payload.responseLatencyMs > 5000

// Timestamp operations
timestamp(attributes.eventTime) > timestamp("2026-01-01T00:00:00Z")
```

CEL expressions are compiled once at config load time and cached.
Evaluation against each event is fast (microseconds for typical expressions).

---

## Plugin System (scafctl-native)

event-reactor uses **scafctl's plugin system directly** -- not a fork or
reimplementation. The same gRPC protocol, the same plugin SDK, the same
plugin binaries.

### How it works

scafctl plugins implement the `ProviderPlugin` interface via gRPC
(hashicorp/go-plugin). event-reactor imports the scafctl plugin SDK and
uses the same `PluginService` protocol:

~~~protobuf
// scafctl proto -- event-reactor reuses as-is
service PluginService {
  rpc GetProviders(Empty) returns (GetProvidersResponse);
  rpc GetProviderDescriptor(ProviderRequest) returns (DescriptorResponse);
  rpc ConfigureProvider(ConfigureRequest) returns (Empty);
  rpc ExecuteProvider(ExecuteRequest) returns (ExecuteResponse);
  rpc ExecuteProviderStream(ExecuteRequest) returns (stream StreamChunk);
  rpc DescribeWhatIf(ExecuteRequest) returns (WhatIfResponse);
  rpc ExtractDependencies(ExecuteRequest) returns (DependenciesResponse);
  rpc StopProvider(ProviderRequest) returns (Empty);
}

service AuthHandlerService {
  rpc GetAuthHandlers(Empty) returns (GetAuthHandlersResponse);
  rpc ConfigureAuthHandler(ConfigureAuthRequest) returns (Empty);
  rpc Login(LoginRequest) returns (stream LoginProgress);
  rpc Logout(AuthHandlerRequest) returns (Empty);
  rpc GetStatus(AuthHandlerRequest) returns (StatusResponse);
  rpc GetToken(TokenRequest) returns (TokenResponse);
  // ... plus token listing, purging, flow detection
}

service HostService {
  rpc GetSecret(SecretRequest) returns (SecretResponse);
  rpc ResolveAuthToken(TokenRequest) returns (TokenResponse);
  rpc Log(LogEntry) returns (Empty);
  // ... plus group queries, config lookups
}
~~~

### What event-reactor adds for listeners

scafctl has no listener concept (it pulls data, it doesn't subscribe to
event streams). event-reactor extends the plugin protocol with a
`ListenerPlugin` interface for event source plugins:

~~~protobuf
// event-reactor addition
service ListenerPluginService {
  rpc GetDescriptor(Empty) returns (ListenerDescriptor);
  rpc Start(ListenerConfig) returns (stream EventEnvelope);
  rpc Stop(Empty) returns (Empty);
  rpc Health(Empty) returns (HealthResponse);
}
~~~

This is the **only new gRPC protocol** event-reactor defines. Everything
else reuses scafctl's protocol.

### Plugin resolution

1. Config references `provider: github` or `type: plugin/kafka-listener`
2. Check scafctl's built-in registry first
3. Check local plugin cache (`~/.er/plugins/`)
4. Resolve from OCI catalog (same catalog format as scafctl)
5. Verify artifact signature (cosign / Sigstore)
6. Launch as subprocess with gRPC transport
7. Register in the provider registry alongside built-ins

### Shared plugin ecosystem

| Plugin | scafctl use | event-reactor use |
|--------|-------------|-------------------|
| `github` | Resolve GitHub data, create PRs/issues | React to events by commenting, labeling, etc. |
| `exec` | Run shell commands in solutions | React to events by executing scripts |
| `env` | Read env vars for solution inputs | Read env vars for reactor config |
| `directory` | List files for scaffolding | (less common, but available) |
| `git` | Git operations in solutions | React by creating branches, commits |
| `secret` | Manage encrypted secrets | Access secrets for reactor auth |

New plugins built for event-reactor (e.g., `smtp`, `webex`, `gcp-pubsub-publish`)
are contributed upstream to the scafctl plugin catalog so both tools benefit.

---

## Project Structure (target)

```
event-reactor/
├── cmd/
│   └── er/                     # CLI entry point
├── pkg/
│   ├── adapter/                # Bridges listeners to matchers/reactors
│   ├── api/                    # HTTP API server (Gin)
│   ├── cel/                    # CEL expression evaluation (wraps scafctl CEL)
│   ├── cli/                    # IOStreams, color output
│   ├── cmd/                    # CLI command wiring (Cobra)
│   │   └── test/               # er test subcommands (match, provider, template, reactor, config)
│   ├── config/                 # Server config loading and validation
│   ├── listener/               # Listener interface + implementations (event-reactor only)
│   │   ├── pubsub/             # GCP Pub/Sub listener
│   │   ├── cloudevents/        # CloudEvents HTTP listener
│   │   ├── webhook/            # Generic webhook listener
│   │   └── plugin/             # Plugin listener adapter (ListenerPluginService)
│   ├── logger/                 # Structured logging (zap + OTEL)
│   ├── matcher/                # CEL-based event matching
│   ├── message/                # Event message types and normalization
│   ├── metrics/                # Prometheus metrics
│   ├── observability/          # OTEL setup (tracing, logging export)
│   ├── params/                 # CLI parameters, settings, version
│   ├── reactor/                # Reactor dispatch (thin layer over scafctl providers)
│   │   └── dispatch.go         # Match event → resolve inputs → call provider.Execute()
│   ├── template/               # Go template rendering (wraps scafctl go-template provider)
│   ├── test/                   # CLI test command implementations
│   │   ├── match.go            # er test match
│   │   ├── provider.go         # er test provider
│   │   ├── template.go         # er test template
│   │   ├── reactor.go          # er test reactor
│   │   └── config.go           # er test config
│   └── mcp/                    # MCP server (reuses pkg/test/ internals)
│       ├── server.go           # MCP server setup (stdio + HTTP/SSE)
│       └── tools.go            # MCP tool definitions and handlers
├── proto/                      # ListenerPluginService proto (event-reactor addition only)
├── k8s/                        # Kubernetes manifests (Kustomize)
│   ├── base/
│   └── overlays/
├── config/                     # Example and default configs
├── docs/                       # Documentation
├── testdata/                   # Sample events for er test commands
└── test/                       # Integration test helpers
```

### What event-reactor builds (scafctl can't do these)

| Package | Purpose |
|---------|--------|
| `pkg/listener/` | Subscribe to event streams -- scafctl has no listener concept |
| `pkg/adapter/` | Wire events through the match → react pipeline |
| `pkg/matcher/` | CEL-based event matching (wraps scafctl CEL evaluation) |
| `pkg/reactor/` | Thin dispatch layer: resolve inputs → call `provider.Execute()` |
| `pkg/api/` | HTTP server for webhook/CloudEvents listeners and health |
| `pkg/config/` | Server config loading, validation, hot-reload |
| `pkg/message/` | Event envelope normalization across sources |
| `pkg/cmd/test/` | `er test` CLI commands |
| `pkg/mcp/` | MCP server for AI-assisted configuration |
| `pkg/observability/` | OTEL tracing, metrics, structured logging setup |

---

## Implementation Phases

### Phase 1 -- Foundation (new project, scafctl integration)

- [ ] Initialize new Go module `github.com/oakwood-commons/event-reactor` (Go 1.22+)
- [ ] Set up [Taskfile](https://taskfile.dev) for build, test, lint, ci
- [ ] Import `scafctl-plugin-sdk` and scafctl built-in provider registry
- [ ] Import scafctl auth handler system
- [ ] Build `pkg/config/` -- server config loading and validation
- [ ] Build `pkg/message/` -- event envelope types and normalization
- [ ] Build `pkg/matcher/` -- CEL-based event matching (wraps scafctl CEL)
- [ ] Build `pkg/reactor/` -- thin dispatch: match → resolve inputs → `provider.Execute()`
- [ ] Build `pkg/adapter/` -- wire listeners to matchers and reactors
- [ ] Build `pkg/listener/pubsub/` -- GCP Pub/Sub pull subscription listener
- [ ] Build `pkg/api/` -- Gin HTTP server with health endpoints
- [ ] Add structured logging with slog/zap + OTEL export
- [ ] Add Prometheus metrics (events received/matched/dropped, provider latency)
- [ ] Add graceful shutdown with drain timeout
- [ ] Build CLI entry point (`cmd/er/`) with Cobra
- [ ] Add Dockerfile (multi-stage, distroless base)
- [ ] Write comprehensive unit tests (target 80%+ coverage)

### Phase 2 -- CLI Testing and MCP Server

- [ ] Implement `er test match` (CEL expression testing against events)
- [ ] Implement `er test provider` (invoke any scafctl provider from CLI)
- [ ] Implement `er test template` (render Go templates against events)
- [ ] Implement `er test reactor` (dry-run or execute a reactor config)
- [ ] Implement `er test config` (validate config file)
- [ ] Implement `er mcp` server (stdio + HTTP/SSE transport)
- [ ] Implement MCP tools: `list_providers`, `get_provider_schema`, `run_provider`
- [ ] Implement MCP tools: `list_listeners`, `get_event_schema`
- [ ] Implement MCP tools: `test_cel_expression`, `render_template`
- [ ] Implement MCP tools: `test_reactor`, `validate_config`, `list_auth_handlers`
- [ ] Add sample event files in `testdata/` for common sources
- [ ] Document CLI testing and MCP setup workflow

### Phase 3 -- Additional Event Sources

- [ ] Implement CloudEvents HTTP listener (CNCF spec)
- [ ] Implement generic webhook listener with HMAC validation
- [ ] Add listener-level metrics and health status
- [ ] Add config hot-reload (file watcher)

### Phase 4 -- New scafctl Plugin Providers

Create these as scafctl plugins (contributed upstream):

- [ ] `smtp` provider -- SMTP email with templated subject/body
- [ ] `webex` provider -- Webex room/direct messaging
- [ ] `gcp-pubsub-publish` provider -- publish to Pub/Sub topics
- [ ] `gcp-secretmanager` provider -- create/update secret versions
- [ ] `slack` provider -- Slack webhook and API messaging
- [ ] `pagerduty` provider -- PagerDuty incident creation
- [ ] `jira` provider -- Jira issue creation and updates

### Phase 5 -- Listener Plugins

- [ ] Define `ListenerPluginService` proto (event-reactor addition)
- [ ] Implement listener plugin manager (launch, health, shutdown)
- [ ] Reuse scafctl OCI catalog for plugin resolution
- [ ] Reuse scafctl Sigstore verification
- [ ] Create example listener plugin (Kafka)
- [ ] Document listener plugin development

### Phase 6 -- Deployment and Operations

- [ ] Add WIF (Workload Identity Federation) support for GCP
- [ ] Add OTEL Collector sidecar manifest for Kubernetes
- [ ] Add Kubernetes base manifests (Kustomize)
- [ ] Add Cloud Run deployment guide
- [ ] Add HPA and PDB manifests
- [ ] Add security hardening (non-root, read-only FS, drop caps)
- [ ] CI/CD pipeline (GitHub Actions: test, lint, build, release)
- [ ] Container image publishing to `ghcr.io/oakwood-commons/event-reactor`

### Phase 7 -- Advanced Features

- [ ] Distributed tracing (OTEL) with configurable export
- [ ] Event batching for high-throughput providers
- [ ] Rate limiting per reactor
- [ ] Circuit breaker for external service calls
- [ ] Event replay / dead-letter reprocessing
- [ ] Web UI for config visualization and event monitoring (stretch)

---

## Technology Choices

| Area | Choice | Rationale |
|------|--------|-----------|
| Language | Go 1.22+ | Cloud-native ecosystem, same as scafctl |
| Build | [Taskfile](https://taskfile.dev) (go-task) | YAML-based, cross-platform, same as scafctl |
| HTTP framework | Gin | Performant, mature, widely adopted |
| CLI framework | Cobra | Standard for Go CLIs, same as scafctl |
| Event filtering | CEL (google/cel-go) | Expressive, type-safe, same as scafctl |
| Templating | Go `text/template` | Shared with scafctl |
| **Provider SDK** | `scafctl-plugin-sdk` | Provides all reactor execution via providers |
| **Auth handlers** | scafctl auth system | GCP, GitHub, Entra ID -- already built |
| **Plugin protocol** | hashicorp/go-plugin (gRPC) | Battle-tested, shared with scafctl |
| Plugin distribution | OCI artifacts + Sigstore | Secure distribution, same as scafctl |
| MCP | Model Context Protocol | AI-assisted config authoring, same as scafctl |
| Logging | zap + OTEL bridge | Structured, high-performance, OTEL-native |
| Metrics | Prometheus (OTEL SDK) | Industry standard, K8s native |
| Tracing | OpenTelemetry | Vendor-neutral, CNCF standard |
| Config | YAML + Viper | Human-readable, env var override support |
| GCP SDK | google-cloud-go | Official client libraries |
| CloudEvents | cloudevents/sdk-go | Official CNCF SDK |
| Testing | testify/assert | Standard Go test assertions |
| Container | Distroless | Minimal attack surface |
| K8s deploy | Kustomize | Simple, no Helm dependency |
| CI/CD | GitHub Actions | Native to GitHub, free for open source |

---

## Non-Goals

- **Not a general-purpose workflow engine** -- event-reactor handles
  event-to-reaction dispatch. For complex multi-step workflows, use scafctl
  solutions directly.
- **Not a message broker** -- event-reactor consumes events, it does not
  provide pub/sub infrastructure.
- **Not a log aggregation platform** -- it filters and forwards, it does
  not store or query logs long-term.
- **Not a FaaS platform** -- unlike Nuclio, event-reactor does not run
  arbitrary user code. Reactions are config-driven provider calls.
- **Don't reimplement what scafctl has** -- if scafctl already has a
  provider or auth handler, import it. If it's missing, create it as a
  scafctl plugin and contribute upstream. Never build a parallel
  implementation in event-reactor.
- **Core binary stays small** -- niche integrations are plugins. Follow
  the scafctl rule: if fewer than 30% of users need it, it's a plugin.
