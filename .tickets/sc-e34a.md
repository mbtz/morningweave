---
id: sc-e34a
status: closed
deps: [sc-03a9]
links: []
created: 2026-01-18T20:24:58Z
type: task
priority: 2
assignee: Marius Holter Berntzen
parent: sc-0ac3
---
# CLI: auth clear command

Add morningweave auth clear <platform|email> to remove stored secrets using the configured provider.

## Acceptance Criteria

- Command validates target key (platform/email) and config path
- Uses secrets provider interface to clear secret values
- Confirms removal in output without leaking secrets
- Returns non-zero exit code on provider error

