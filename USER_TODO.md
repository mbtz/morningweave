# MorningWeave USER_TODO

## Development
- Run PRD v1 end-to-end verification (ticket `a-2e91`): run the full cycle on macOS + home server (where keys exist), record timings/memory/warnings, validate digest word cap + item count, and update `docs/prd-acceptance.md` with results.
- Commit and close ticket `a-f870` (git index.lock blocked here): `git add internal/cli/cli.go internal/runner/run.go internal/runner/auth_requirements.go internal/runner/auth_requirements_test.go .tickets/a-f870.md USER_TODO.md && git commit -m "Add auth requirement hints for missing credentials"`; then ` /opt/homebrew/bin/tk close a-f870` and commit the ticket status update if needed.
