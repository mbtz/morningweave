---
id: sc-be13
status: closed
deps: []
links: []
created: 2026-01-19T12:18:55Z
type: task
priority: 2
assignee: Marius Holter Berntzen
parent: sc-434f
---
# Spec: Security & privacy requirements

Verify secrets handling, redaction, and local-only guarantees match PRD; document gaps.

## Acceptance Criteria

- Secrets stored via keychain/1Password with plaintext fallback; never printed or echoed by CLI.
- `auth get` reports status without revealing secret values.
- Logs redact sensitive values and avoid outbound telemetry.
- Plaintext secret usage emits clear warnings or TODO guidance.
- Documentation covers privacy expectations and local-only processing.
