---
description: "event-reactor: Generate a conventional commit message from staged or recent changes."
agent: "commit-message"
---
Analyze the current changes and generate a conventional commit message. **Output the message only -- do not commit.**

## Steps

1. Check `git diff --cached --stat` for staged changes (fall back to `git diff --stat`)
2. Read the actual diff to understand what changed
3. If asked to amend, check `git log -1` for the last commit
4. Generate a message: `<type>(<scope>): <description>` + body with bullet points summarizing key changes
5. Output in a code block for the user to copy

## Format rules

- Types: feat, fix, docs, perf, refactor, style, test, chore, ci, revert
- Always include a body unless the change modifies only one file with a non-functional edit (formatting, whitespace, comment-only).
- Add `!` and `BREAKING CHANGE:` footer for breaking changes.
