---
title: "event-reactor"
type: docs
---

# event-reactor

Event-driven automation engine. Listens for events, matches them with CEL expressions, and dispatches reactions via pluggable providers.

{{< columns >}}

## Listen

Accept events from HTTP push, CloudEvents, webhooks, and Pub/Sub sources.

<--->

## Match

Filter events using [CEL](https://github.com/google/cel-go) expressions with compiled caching.

<--->

## React

Dispatch to pluggable providers (echo, http, exec, log) with templated inputs and auth injection.

{{< /columns >}}

## Documentation

- [Getting Started](getting-started/) -- run event-reactor and react to your first event.
- [API Reference](api/) -- endpoints, request formats, and the event model.
- [CLI Reference](cli/) -- the `er` command and its subcommands.
- [Operations](operations/) -- deployment, health checks, and verifying signed artifacts.
- [Design & Architecture](design/) -- architecture, configuration, providers, and auth.

