---
id: sc-4cd0
status: closed
deps: []
links: []
created: 2026-01-19T12:18:45Z
type: task
priority: 2
assignee: Marius Holter Berntzen
parent: sc-434f
---
# Spec: Scheduler + per-tag/category schedules

Validate scheduler behavior, per-tag/category cron overrides, and start/stop flows vs PRD.

## Acceptance Criteria

- Scheduler respects global default schedule (daily early morning local time) and per-tag/category overrides.
- One digest is emitted per tag/category schedule window; local time zone is honored.
- `start --headless` runs without prompts; `stop` shuts down cleanly.
- Cron fallback output matches configured schedules when daemon mode is not used.
- `status` reports next run windows for tags/categories.
