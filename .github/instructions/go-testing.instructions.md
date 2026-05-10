---
description: "Go testing conventions for event-reactor: table-driven tests, testify/assert, race detection, and coverage. Use when writing or editing Go test files."
applyTo: "**/*_test.go"
---

# Go Testing Conventions

## Framework

- Use standard `go test` with **table-driven tests**
- Use `testify/assert` for assertions

## Race Detection

Always run with the `-race` flag:

```bash
go test -race ./...
```

or use `task test:race`.

## Coverage

```bash
go test -cover ./...
```

### Coverage Targets

| Code Type | Target |
|-----------|--------|
| Domain packages (`pkg/...`) | 80%+ |
| CLI commands (`pkg/cmd/...`) | 65%+ |
| Critical business logic | 90%+ |

## Reference

See skill: `golang-testing` for testing patterns and coverage.
