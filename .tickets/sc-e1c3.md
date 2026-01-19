---
id: sc-e1c3
status: closed
deps: []
links: []
created: 2026-01-19T12:18:50Z
type: task
priority: 2
assignee: Marius Holter Berntzen
parent: sc-434f
---
# Spec: Storage, logging, and retention

Confirm SQLite schema, run logs, and retention windows align with PRD.

## Acceptance Criteria

- SQLite schema covers runs, seen items, dedupe cache, and tag weights; database stored under `data/`.
- Log retention defaults to 30 days and seen retention to 45 days (configurable).
- `logs` surfaces run status, per-platform counts, email sent flag, and supports `--since` and `--json`.
- Run log records start/end time, structured status (success/empty/error), and failure details.
