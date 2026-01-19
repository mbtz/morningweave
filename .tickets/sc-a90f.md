---
id: sc-a90f
status: closed
deps: []
links: []
created: 2026-01-19T11:45:57Z
type: bug
priority: 1
assignee: Marius Holter Berntzen
---
# Fix storage run queries NULL scope handling

Handle NULL scope_type values and missing rows in internal/storage run queries (RunCRUD/GetLastRunForScope/PruneRunsBefore). Use sql.NullString or COALESCE and return nil/zero values when rows missing.

