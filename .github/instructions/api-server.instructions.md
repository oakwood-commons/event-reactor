---
description: "API server layer rules for event-reactor. Endpoints are thin handlers -- no business logic. Use Gin for routing, validate inputs, test with test files. Use when editing API server packages."
applyTo: "pkg/api/**/*.go"
---

# API Server Layer

API endpoints are **thin handlers** -- they validate input, call domain packages, and return responses.

## Rules

- **No business logic** -- delegate to packages in `pkg/`
- Use Gin for routing and middleware
- Validate request inputs before processing
- Return structured error responses
- Always add new endpoints to API tests
- Use `IOStreams` from `pkg/cli/` for any CLI-facing output, not bare `fmt.Fprintf`
