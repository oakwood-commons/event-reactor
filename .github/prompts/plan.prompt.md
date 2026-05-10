---
description: "event-reactor: Create an implementation plan for a feature. Produces a structured blueprint with architecture decisions, task breakdown, and testing strategy."
agent: "agent"
argument-hint: "Describe the feature to plan (e.g., 'Add Webex reactor')"
---
Create a structured implementation blueprint for the described feature:

1. **Summary** -- What and why
2. **Architecture decisions** -- Layers affected, new types, interface changes
3. **Task breakdown** -- Ordered steps with files, complexity, dependencies
4. **Interface design** -- Define contracts first
5. **Error handling** -- Sentinel errors, wrapping strategy
6. **Testing strategy** -- Unit tests, integration tests
7. **Documentation** -- Docs, examples
8. **Risks & edge cases** -- What could go wrong

Follow event-reactor conventions: listener/matcher/reactor architecture, CEL for filtering, Go templates for message rendering.
