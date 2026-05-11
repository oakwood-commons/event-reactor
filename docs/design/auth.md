---
title: "Auth System"
weight: 2
---

# Auth System

event-reactor's auth system handles two concerns:

1. **Inbound validation** -- Verifying webhook signatures (HMAC-SHA256)
2. **Outbound token generation** -- Injecting authorization headers into provider calls

## Auth Handlers

Auth handlers are named token generators configured in the server config. Reactors reference them by name via the `auth` field.

```yaml
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
        token: xoxb-my-bot-token

    - name: gcp
      type: service-account
      defaultScopes:
        - https://www.googleapis.com/auth/cloud-platform
      config:
        keyFile: /etc/secrets/sa-key.json
```

### Supported Types

| Type | Description | Token Format |
|------|-------------|-------------|
| `static-token` | Fixed bearer token | `Bearer <token>` |
| `github-token` | GitHub PAT | `token <pat>` |
| `github-app` | GitHub App installation token | `token <installation-token>` |
| `oauth2-client-credentials` | OAuth2 client credentials flow | `Bearer <access-token>` |
| `service-account` | GCP service account key | `Bearer <access-token>` |

### Token Injection Flow

```
Reactor Config (auth: "github")
       |
       v
Auth Registry.GetToken(ctx, "github")
       |
       v
Handler generates/refreshes token
       |
       v
Token injected via context
       |
       v
HTTP Provider sets Authorization header
```

## Webhook Secrets

Inbound webhook signatures are validated per-source:

```yaml
auth:
  webhookSecrets:
    - source: github
      secret: whsec_abc123def456
```

The webhook handler checks `X-Hub-Signature-256` (or `X-Signature-256`) against the HMAC-SHA256 of the request body. Requests with invalid signatures are rejected with 401.

## Security Considerations

- Auth handler configs (tokens, keys) should be provided via environment variables or mounted secrets, not committed to source control
- Tokens are passed via context, never exposed in provider inputs or outputs
- The circuit breaker protects against cascading auth failures
- Service account key files should use Workload Identity Federation when possible
