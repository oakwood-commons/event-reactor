---
title: "Operations"
weight: 5
---

# Operations

This page covers running event-reactor in production: deployment, ports, health
checks, configuration, and verifying the artifacts you deploy.

## Deploy to Kubernetes

The repository ships Kustomize manifests under `deploy/`:

```bash
# Base (single replica, ClusterIP service, ConfigMap-mounted config)
kubectl apply -k deploy/base

# Production overlay (adds HPA and PodDisruptionBudget, patches the deployment)
kubectl apply -k deploy/production
```

The base deployment runs as a non-root user (`65534`) with a read-only root
filesystem, all capabilities dropped, `allowPrivilegeEscalation: false`, and the
`RuntimeDefault` seccomp profile. The container reads its config from
`/etc/event-reactor/config.yaml`, mounted read-only from the
`event-reactor-config` ConfigMap.

## Ports

| Port | Name | Status |
|------|------|--------|
| `8080` | `http` | HTTP API and health endpoints (from `server.port`). |
| `9090` | `metrics` | Reserved. Declared in the manifests and config (`server.metricsPort`, default `9090`) but **no metrics endpoint is served yet**. |

> Metrics and tracing: the config schema accepts `observability.metrics` and
> `observability.tracing`, but the exporters are not wired in the current
> release. Only structured logging and the health endpoints are served today.
> Do not rely on a `/metrics` endpoint being present.

## Health checks

Two probe endpoints are always served, at paths from `server.healthCheck`
(defaults shown):

| Probe | Default path | Config key |
|-------|--------------|------------|
| Liveness | `/health/live` | `server.healthCheck.liveness` |
| Readiness | `/health/ready` | `server.healthCheck.readiness` |

The base deployment wires these as Kubernetes `livenessProbe` and
`readinessProbe` against the `http` port.

## Configuration and secrets

- **Config file.** Provide the YAML config via a mounted file (ConfigMap in
  Kubernetes) and point `--config` at it. See
  [Configuration](../design/configuration/) for the full schema.
- **Hot-reload.** The server watches the config file and reloads reactors and
  listeners on change (fsnotify), so a ConfigMap update is picked up without a
  restart.
- **Secrets.** Reactor inputs can be pulled from GCP Secret Manager
  (`valueFrom.secretKeyRef`), environment variables (`fromEnv`), or files
  (`fromFile`). Prefer these over inlining secrets in the config. Webhook HMAC
  secrets live under `auth.webhookSecrets`; source them from a mounted secret
  rather than committing them.

## Logging

Structured logging is controlled by `observability.logging`:

```yaml
observability:
  logging:
    level: info   # debug | info | warn | error
    format: json  # json | text
```

Use `json` in production for machine-parseable logs; `text` is convenient for
local runs.

## Verify signed artifacts

Releases are signed with [cosign](https://docs.sigstore.dev/) keyless signing
(GitHub OIDC), so you can verify provenance before deploying.

Verify a downloaded checksums file:

```bash
cosign verify-blob \
  --certificate event-reactor_<version>_SHA256SUMS.pem \
  --signature  event-reactor_<version>_SHA256SUMS.sig \
  --certificate-identity-regexp 'https://github.com/oakwood-commons/event-reactor/.*' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  event-reactor_<version>_SHA256SUMS
```

Verify a container image:

```bash
cosign verify \
  --certificate-identity-regexp 'https://github.com/oakwood-commons/event-reactor/.*' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  ghcr.io/oakwood-commons/event-reactor:latest
```

See the [README](https://github.com/oakwood-commons/event-reactor#install) for
installation details and image tags (`latest`, `X.Y.Z`, `X.Y`).
