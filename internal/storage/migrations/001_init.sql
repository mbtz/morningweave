-- Initial schema for MorningWeave storage.

CREATE TABLE IF NOT EXISTS schema_migrations (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  applied_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS runs (
  id INTEGER PRIMARY KEY,
  started_at INTEGER NOT NULL,
  finished_at INTEGER,
  status TEXT NOT NULL,
  scope_type TEXT,
  scope_name TEXT,
  items_fetched INTEGER NOT NULL DEFAULT 0,
  items_ranked INTEGER NOT NULL DEFAULT 0,
  items_sent INTEGER NOT NULL DEFAULT 0,
  email_sent INTEGER NOT NULL DEFAULT 0,
  error TEXT,
  platform_counts TEXT,
  created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX IF NOT EXISTS runs_started_at_idx ON runs (started_at);

CREATE TABLE IF NOT EXISTS seen_items (
  id INTEGER PRIMARY KEY,
  canonical_url TEXT NOT NULL,
  title TEXT,
  first_seen_at INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL,
  source_platform TEXT,
  source_type TEXT,
  source_identifier TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS seen_items_canonical_idx ON seen_items (canonical_url);
CREATE INDEX IF NOT EXISTS seen_items_last_seen_idx ON seen_items (last_seen_at);

CREATE TABLE IF NOT EXISTS dedupe_map (
  canonical_url TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  normalized_title TEXT NOT NULL,
  last_seen_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS dedupe_map_last_seen_idx ON dedupe_map (last_seen_at);

CREATE TABLE IF NOT EXISTS tag_weights (
  tag_name TEXT PRIMARY KEY,
  weight REAL NOT NULL,
  runs INTEGER NOT NULL DEFAULT 0,
  hits INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS tag_weights_updated_idx ON tag_weights (updated_at);
