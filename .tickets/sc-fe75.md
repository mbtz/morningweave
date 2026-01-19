---
id: sc-fe75
status: closed
deps: []
links: []
created: 2026-01-19T12:18:38Z
type: task
priority: 1
assignee: Marius Holter Berntzen
parent: sc-434f
---
# Spec: Platform connector access handling

Ensure official API usage, access warnings, and disable flows align with PRD per platform.

## Acceptance Criteria

- Each connector uses official APIs and documents required scopes/notes.
- Missing credentials or access limits trigger clear warnings and platform disablement; `status` surfaces disabled platforms.
- USER_TODO guidance includes platform-specific access steps and scope hints.
- Reddit supports subreddits, users, and keyword searches.
- X supports keyword queries and user timelines/lists (subject to API tier).
- Instagram supports accounts and hashtags (Business/Creator token prerequisites noted).
- HN supports top/new/best lists with optional keyword filtering.
