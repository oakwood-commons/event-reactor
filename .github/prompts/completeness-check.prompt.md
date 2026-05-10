---
description: "event-reactor: Check if staged changes have corresponding docs, tests, and examples."
agent: "agent"
argument-hint: "Optional: specific area to check"
---
Review staged changes and check if supporting artifacts exist:

1. Run `git diff --cached --stat` to identify staged changes
2. If nothing is staged, fall back to `git log origin/main..HEAD --stat` to check pushed commits on the branch
3. For each feature, provider, or command, verify:
   - Docs in `docs/` or `docs/design/`
   - Unit tests alongside each package
   - Integration/smoke tests in `test/`
   - Config examples in test data (`pkg/config/testdata/`, `test/testdata/`)
4. Report present vs missing as a checklist
5. Do not create anything, just report the gaps
