---
id: sc-de24
status: closed
deps: []
links: []
created: 2026-01-19T12:29:03Z
type: task
priority: 1
assignee: Marius Holter Berntzen
parent: sc-434f
---
# Spec: Run pipeline & execution flow

Define per-run data flow (config/creds -> fetch -> filter -> score -> dedupe -> select -> email/log) and non-functional expectations per PRD.

## Acceptance Criteria

- One-shot run loads config + creds and fetches recent items per source using last-run window or 24h default.
- Language filter supports en/no with confidence threshold; low-confidence items are dropped.
- Scoring + dedupe follow PRD rules; top 5-10 items selected overall (not per platform) with 300-350 word cap applied.
- Transient API errors retry with backoff; partial success still emits digest when possible.
- Run results log success/empty/error, per-platform counts, and whether email was sent.
- Non-functional targets are documented: <60s typical run time and <200MB peak memory.
