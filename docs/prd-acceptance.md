# MorningWeave PRD v1 Acceptance Checklist

Checklist mapped to PRD sections. Record dates, results, and notes per item.

## Test context
- Date:
- Machine(s):
- OS + shell:
- Go version:
- Config profile:
- Enabled platforms:

## 1. CLI surface (PRD 8)
- [ ] `morningweave init` creates `config.yaml` in repo root and `data/` with SQLite store; prompts for email provider.
  - Expected: config template with globals, tags/categories, platforms, email, logging.
- [ ] `morningweave add-platform <name>` enables platform and prompts for sources + creds.
  - Expected: config updated; missing/invalid credentials produce actionable guidance.
- [ ] `morningweave set-tags` / `set-category` updates keywords, schedule, weights.
  - Expected: config reflects new tags/categories.
- [ ] `morningweave run [--tag|--category]` runs a single worker.
  - Expected: logs show counts per platform; digest sent (or skipped if empty).
- [ ] `morningweave start --headless` / `stop` manage background scheduler.
  - Expected: start without prompts; stop shuts down cleanly.
- [ ] `morningweave status` shows enabled platforms, next runs, last run result.
  - Expected: warnings for missing creds/empty sources without crashing.
- [ ] `morningweave logs [--since] [--json]` shows recent runs.
  - Expected: success/empty/error + per-platform counts + email sent flag.
- [ ] `morningweave test-email` sends sample digest.
  - Expected: provider-specific success message.
- [ ] `morningweave auth set|get|clear <platform|email>` manages secrets.
  - Expected: no secret values are printed; clear removes stored secret.

## 2. Scheduling & execution (PRD 9, 10)
- [ ] Default daily schedule runs at configured local time.
- [ ] Per-tag/category schedules override defaults.
- [ ] One digest is emitted per tag/category window.
- [ ] `run` uses last-run window (or 24h default when no history).
- [ ] Background scheduler handles due runs and allows clean stop.

## 3. Platforms & sources (PRD 5)
- [ ] Reddit: supports subreddits, users, keyword searches.
- [ ] X: supports keyword queries, user timelines/lists (tier-aware).
- [ ] Instagram: supports accounts and hashtags; enforces Business/Creator prereq.
- [ ] Hacker News: supports top/new/best with optional keyword filter.
- [ ] Platforms without API access are disabled with clear warnings.

## 4. Configuration & secrets (PRD 6, 7)
- [ ] `config.yaml` includes globals (schedule, language, word cap 300-350, max items 10).
- [ ] Tag/category fields include keywords, optional weights, schedule override, language, recipients.
- [ ] Platform config includes enabled flag, credentials reference, sources, weights.
- [ ] Email config supports Resend + SMTP with from/to and subject template.
- [ ] Logging config includes level and retention days.
- [ ] Secrets stored in keychain/1Password when available; fallback to YAML with warning.

## 5. Relevance, ranking, and dedupe (PRD 11, 12)
- [ ] Keyword/tag match uses stemming; language filter allows en/no only.
- [ ] Score combines recency decay, engagement, tag match, and source weights.
- [ ] Adaptive tag weights adjust only after >=10 runs and persist.
- [ ] URL canonicalization drops fragments, tracking params, default ports; normalizes host/path.
- [ ] Title-based fuzzy dedupe merges duplicates and retains source list.
- [ ] Top 5-10 items selected overall (not per platform).

## 6. Email delivery (PRD 13)
- [ ] HTML digest is mobile-friendly, light on images.
- [ ] Total word count stays within 300-350; max items 10.
- [ ] Each item includes title, 1-2 sentence excerpt, and source list.
- [ ] No email is sent when zero items; logged as empty-run.
- [ ] Resend works as first-class; SMTP fallback verified (e.g., Outlook).

## 7. Storage, logging, and retention (PRD 14, 15)
- [ ] SQLite stores runs, seen items, dedupe cache, tag weights under `data/`.
- [ ] Run logs include start/end, status, per-platform counts, email sent flag.
- [ ] `logs` output redacts secrets.
- [ ] Retention: logs default 30 days; seen items default 45 days (configurable).

## 8. Non-functional targets (PRD 17)
- [ ] Typical run completes in <60s with <=4 sources per platform.
- [ ] Peak memory usage <200MB.
- [ ] Transient API errors retry with backoff; partial success allowed.

## 9. Security & privacy (PRD 16)
- [ ] No outbound telemetry.
- [ ] Secrets never printed or stored in logs.
- [ ] Local-only data processing and storage.

## Evidence log
- Notes:
- Screenshots/logs:
- Links to run artifacts:
