---
description: "event-reactor: Run Go code review on recent changes. Checks for idiomatic Go, security, error handling, concurrency, and project conventions."
agent: "go-reviewer"
---
Review the current Go code changes thoroughly. You MUST complete ALL phases below. Do not stop after finding a few issues.

## Phase 1: Automated checks

1. Run `go vet ./...`
2. Run `git diff --stat HEAD -- '*.go'` and `git status --short` to identify all changed/new files
3. Read the full diff for all changed files
4. Read the full contents of all new (untracked) files
5. Run `go test -coverprofile` on **every** changed package
6. Run `go test -race` on changed packages

## Phase 2: Systematic review (check EVERY item)

For each changed/new file, check ALL of these categories. Do not skip any.

### Security
- [ ] Command injection (user input passed to exec without sanitization)
- [ ] Path traversal (user-controlled paths not validated for containment)
- [ ] Hardcoded secrets, tokens, or credentials
- [ ] Unsafe deserialization of untrusted input

### Error handling
- [ ] Ignored errors (unchecked error returns, `_ = someFunc()`)
- [ ] Missing error wrapping (`fmt.Errorf("context: %w", err)`)
- [ ] Panics used for recoverable errors
- [ ] Error messages that leak sensitive information

### Concurrency
- [ ] Goroutine leaks (goroutines that never exit)
- [ ] Race conditions (shared state without synchronization)
- [ ] Deadlock potential (inconsistent lock ordering)

### Code quality
- [ ] Functions over 60 lines (flag, suggest extraction)
- [ ] Nesting depth over 4 levels
- [ ] Non-idiomatic Go patterns

### event-reactor conventions
- [ ] Terminal output uses `IOStreams` from `pkg/cli/` (never bare `fmt.Fprintf`)
- [ ] Business logic is NOT in CLI command wiring or API handlers
- [ ] No magic values (use constants)

### Naming
- [ ] New public symbols follow Go conventions and project patterns
- [ ] Input/output field names are clear and unambiguous
- [ ] Consistent naming with similar existing features

### Correctness
- [ ] Delegation: when creating temporary structs to delegate, ALL fields used by the callee are forwarded
- [ ] Mutation safety: no mutation of shared/input structs; prefer passing overrides
- [ ] Edge cases: nil inputs, empty slices, zero values handled
- [ ] Map iteration: output built from map ranges must sort keys for deterministic ordering
- [ ] `defer cancel()` placed immediately after context creation, before any early returns

### Dead code
- [ ] New exported functions have callers outside test files (use `grep` to verify)
- [ ] New struct fields are read/written somewhere (use `grep` to verify)
- [ ] No orphaned imports after refactoring

## Phase 3: Adversarial analysis

For each new feature or behavioral change, actively try to break it:
- What happens with nil/empty/zero inputs?
- What happens if a dependency is missing or returns an error?
- What happens under concurrent access?
- Can this change cause a regression in existing behavior?

## Phase 4: Cross-file consistency

- [ ] Changes to types/interfaces are reflected in all implementations
- [ ] Changes to function signatures are reflected in all call sites
- [ ] New context values have matching With*/FromContext pairs AND tests

## Phase 5: Coverage analysis

1. Run `go test -coverprofile=cover.out ./path/to/changed/pkg/...` for each changed package
2. Run `go tool cover -func=cover.out` to get per-function coverage
3. For **every** changed file:
   - Flag any changed function with coverage below 70%
   - Flag any NEW file with overall coverage below 70%
   - Flag any file with 0% patch coverage as **HIGH** severity
4. For each gap, recommend specific test cases (function name, inputs, expected behavior)

## Phase 6: Self-review (MANDATORY, do not skip)

After completing phases 1-5, review your own findings:
1. Re-read the full diff one more time
2. For each file you reviewed, ask: "What did I NOT check?"
3. For each finding you reported, ask: "Is this the root cause or just a symptom?"
4. Look for patterns: if you found one delegation bug, are there OTHER delegations with the same problem?
5. Check: did you verify every item in the Phase 2 checklist? If you skipped any, go back now.

Report any additional findings from the self-review as a separate section.

## Output format

Use severity levels: CRITICAL > HIGH > MEDIUM > LOW > INFO
For each finding include: file, line, severity, description, and suggested fix.
End with a summary table: files reviewed, findings by severity, coverage status.
