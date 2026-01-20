---
id: a-6b5f
status: closed
deps: []
links: []
created: 2026-01-19T15:38:16Z
type: bug
priority: 1
assignee: Marius Holter Berntzen
---
# Fix X credentials parsing for raw tokens

Allow X credentials to be provided as a raw bearer token (no key/value) and accept common key aliases like x-api-key. Add unit tests for ParseCredentials to cover raw token and key alias parsing so 1Password/keychain values work.

