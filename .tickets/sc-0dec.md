---
id: sc-0dec
status: closed
deps: []
links: []
created: 2026-01-19T11:45:36Z
type: bug
priority: 1
assignee: Marius Holter Berntzen
---
# Fix Reddit connector compile errors from USER_FEEDBACK

Resolve build errors in internal/connectors/reddit/reddit.go (sourceEndpoint type mismatch, accessToken field/method collision, redeclared sourceEndpoint). Ensure reddit package builds and tests run.

