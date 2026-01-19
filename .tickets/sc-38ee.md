---
id: sc-38ee
status: closed
deps: []
links: []
created: 2026-01-19T12:18:32Z
type: task
priority: 1
assignee: Marius Holter Berntzen
parent: sc-434f
---
# Spec: Config schema + defaults parity

Validate config.yaml fields/defaults match PRD and update docs/validation as needed.

## Acceptance Criteria

- `config.yaml` lives in project root and includes global defaults: schedule, language filter (en/no), digest word cap (300-350), and max items (10).
- Tags/categories define name, keywords, optional weights, schedule override, language, and recipient list.
- Platforms define enabled flag, credential reference, sources per platform type, and per-platform + per-source weight/priority (default 1.0).
- Email config includes provider (Resend/SMTP), from/to, and subject template fields; logging config includes log level and retention days.
- Secrets can reference keychain/1Password entries with plaintext fallback documented/validated.
