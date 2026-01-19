---
id: sc-0ef1
status: closed
deps: [sc-03a9]
links: []
created: 2026-01-18T20:24:54Z
type: task
priority: 2
assignee: Marius Holter Berntzen
parent: sc-0ac3
---
# CLI: auth get command

Add morningweave auth get <platform|email> to resolve secrets via the configured provider and report status without printing secret values.

## Acceptance Criteria

- Command validates target key (platform/email) and config path
- Uses secrets provider interface to read secret metadata
- Prints clear status (found/missing + reference) without revealing secret value
- Returns non-zero exit code on missing or provider error

