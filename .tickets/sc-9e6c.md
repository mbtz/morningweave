---
id: sc-9e6c
status: closed
deps: []
links: []
created: 2026-01-18T23:41:53Z
type: task
priority: 2
assignee: Marius Holter Berntzen
---
# CLI: status health checks

Extend `morningweave status` to surface configuration health warnings: missing email provider/from/to, missing credentials refs or missing secrets for enabled platforms, and enabled platforms with empty sources. Use secrets.Store.Inspect for refs (no secret values), and emit warnings instead of failing. (Epic: CLI and UX flow)
