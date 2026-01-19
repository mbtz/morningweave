---
id: sc-1ecc
status: closed
deps: []
links: []
created: 2026-01-19T11:40:31Z
type: bug
priority: 1
assignee: Marius Holter Berntzen
---
# Fix storage run scan for nullable scope

Handle NULL scope_type in internal/storage run queries (use sql.NullString or COALESCE) and adjust tests for missing scope lookups.

