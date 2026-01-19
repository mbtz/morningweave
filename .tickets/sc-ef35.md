---
id: sc-ef35
status: closed
deps: []
links: []
created: 2026-01-19T11:40:29Z
type: bug
priority: 2
assignee: Marius Holter Berntzen
---
# Fix dedupe canonicalization default port

Update internal/dedupe URL canonicalization to strip default ports (http:80/https:443) and satisfy TestCanonicalizeURL expectations.

