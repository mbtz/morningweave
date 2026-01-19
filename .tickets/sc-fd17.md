---
id: sc-fd17
status: closed
deps: []
links: []
created: 2026-01-19T12:18:42Z
type: task
priority: 1
assignee: Marius Holter Berntzen
parent: sc-434f
---
# Spec: Ranking + dedupe rules alignment

Verify ranking signals, adaptive weights, and dedupe rules match PRD; add tests/docs as needed.

## Acceptance Criteria

- Ranking uses recency decay, engagement, tag/keyword match (stemmed), source weight, and language match signals.
- Adaptive tag/category weights update after >=10 runs using per-tag success rate and persist in storage (no user prompt).
- Dedupe canonicalizes URLs (lowercase scheme/host, drop fragments, default ports, normalize paths, strip utm_*, gclid, fbclid, igshid).
- Fuzzy title similarity merges duplicates; merged item retains all source links and prefers highest-engagement metadata.
- Tests cover canonicalization and dedupe merge behavior.
