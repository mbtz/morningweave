# MorningWeave: Async Content Digest CLI — PRD v1

## 1) Goal
MorningWeave is a single-user CLI that gathers content from multiple platforms via official APIs, ranks by relevance to configured tags/categories, dedupes across platforms, and emails a concise HTML digest on a schedule or on demand.

The tool should be written in golang

## 2) In Scope (v1)
- Platforms: Reddit, X (x.com), Instagram, Hacker News — official APIs only. If API access is unavailable, the platform is disabled with a clear CLI warning.
- Surfaces: broad categories, keywords/tags, usernames/accounts, communities/subreddits/hashtags/lists.
- Execution: one-shot runs and background mode (`start --headless`) via daemon/cron.
- Output: 5–10 top items overall (not per platform) with combined source links; skip email if no items.
- Config: YAML file stored locally; per-tag/category schedules; platform creds stored locally.
- Migration: replace the existing Python implementation with Go, preserving current CLI scaffolding behavior (init, add-platform, set-tags/set-category) before new features.

## 3) Out of Scope (v1)
- Any scraping outside official APIs.
- Multi-user tenancy.
- Mobile clients or GUI.
- Non-English/Norwegian content.

## 4) User & Environment
- Single user on macOS (zsh); should also run on a home server.
- Fast setup (<10 minutes per platform if keys are available).

## 5) Platforms & Source Definition
- **Reddit**: subreddits, users, keyword searches.
- **X**: keyword queries and user timelines/lists (subject to API tier).
- **Instagram**: accounts and hashtags (requires Business/Creator + appropriate token).
- **Hacker News**: top/new/best + optional keyword filter over recent items.
- Each platform module documents required scopes; missing keys generate actionable TODO entries.

## 6) Configuration (YAML)
- Paths: `config.yaml` in project root; secrets can reference keychain/1Password entries.
- Global: default schedule, language filter (en/no), digest word cap (300–350), max items (10).
- Tags/Categories: name, keywords, optional weights, schedule override, language, recipient(s).
- Platforms: enabled flag, credentials reference, list of sources (per platform type), optional per-platform and per-source weight/priority values (default 1.0) used by ranking.
- Email: provider config (Resend or SMTP), from, to, subject template.
- Logging: log level, retention days.

## 7) Credentials & Secrets
- Preferred storage: OS keychain or 1Password; fallback to local YAML secrets section.
- CLI offers `auth set <platform>` to store/retrieve; never prints secrets back.
- `USER_TODO.md` is auto-maintained with step-by-step key acquisition guides per platform/provider.

## 8) CLI Surface (proposed)
- `init` — generate `config.yaml` + empty `USER_TODO.md`, initialize the SQLite data store under `data/`, and prompt for email provider.
- `add-platform <name>` — enable platform and prompt for sources/creds.
- `set-tags` / `set-category` — add/update tag/category definitions (keywords, schedule, weights).
- `run [--tag <tag>|--category <cat>]` — one-shot worker.
- `start --headless` / `stop` — manage background scheduler (daemon/cron abstraction).
- `status` — show next runs, enabled platforms, last run result.
- `logs [--since <time>]` — tail/audit runs (success/empty/error, counts per platform).
- `test-email` — send sample digest to verify provider.
- `auth set|get|clear <platform|email>` — manage secrets.

## 9) Scheduling & Execution
- Default daily run early morning local time (configurable cron spec).
- Per-tag/category schedules supported; emits one digest per tag/category schedule window.
- Background mode: long-running process with internal scheduler; cron fallback if daemon not chosen.

## 10) Data Flow (per run)
1) Load config + creds.
2) Fetch recent items per platform source (time window since last run or 24h default).
3) Filter by language (en/no) and tags/categories.
4) Score items (recency + engagement + tag match + source weight).
5) Dedupe across platforms by normalized URL and fuzzy title; merged sources list.
6) Pick top 5–10 overall; cap total words at 300–350.
7) Render HTML email; if empty, skip send.
8) Log run outcome; persist seen items for dedupe.

## 11) Relevance & Ranking (v1)
- Signals: keyword/tag match (stemmed), platform engagement (votes/likes/reposts/views), recency decay, source priority (user-defined), language match.
- Adaptive weights: after >=10 runs, compute per-tag success rate and adjust tag weights slightly (LLM/light heuristic pass; no user prompt).
- Fallback when engagement absent: rely on recency + tag match.

## 12) Dedupe Rules
- Primary key: canonicalized URL; secondary: fuzzy title similarity.
- Canonicalization: lowercase scheme/host, drop fragments, remove default ports, normalize path, and strip common tracking params (utm_*, gclid, fbclid, igshid).
- Merged item retains all source links (list in email).
- Prefer the highest-engagement variant’s metadata for title/excerpt.

## 13) Email Delivery
- Provider: Resend first-class; SMTP fallback (works with Outlook). Both configurable in YAML.
- Format: clean HTML, mobile-friendly, light on images, 300–350 words total.
- Each item: title, 1–2 sentence excerpt, source list, platform badges.
- No email if zero items; log as “empty-run”.

## 14) Storage & State
- SQLite for runs, seen items, dedupe cache, adaptive weights.
- Files: `config.yaml`, `USER_TODO.md`, `data/` (SQLite, logs).
- Retention: logs 30 days (configurable); seen items 45 days.

## 15) Logging & Observability
- Structured logs per run: start/end time, status (success/empty/error), counts per platform, failures, email sent? (Y/N).
- `logs` command surfaces recent runs; optional `--json`.

## 16) Security & Privacy
- Secrets stored in keychain/1Password when available; otherwise local file with clear warning.
- No outbound telemetry; all data local.
- Redact secrets in logs.

## 17) Non-Functional Targets
- Setup: <10 minutes per platform if keys ready.
- Runtime: <60s per run for typical config (≤4 sources per platform).
- Memory: <200MB peak.
- Reliability: retries with backoff on transient API errors; partial success allowed.

## 18) Risks & Mitigations
- API access limits (X/Instagram): detect tier limits and surface in `status`; allow per-platform disable.
- Resend/SMTP misconfig: `test-email` + TODO guidance.
- Language filter accuracy: simple lang-id with confidence threshold; drop low-confidence items.
- Instagram token prerequisites: document Business/Creator requirement in TODO.

## 19) Future (post-v1)
- Multi-user / multi-tenant profiles.
- Web UI for logs and config.
- More providers (RSS, LinkedIn, YouTube).
- Advanced ML ranking; user feedback loop on email clicks (if ever tracked).
