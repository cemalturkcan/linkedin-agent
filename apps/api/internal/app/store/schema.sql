CREATE TABLE model_calls (
  id                 INTEGER PRIMARY KEY AUTOINCREMENT,
  purpose            TEXT NOT NULL,
  model              TEXT NOT NULL DEFAULT '',
  tool_name          TEXT NOT NULL DEFAULT '',
  state              TEXT NOT NULL DEFAULT 'running',
  started_at         TEXT NOT NULL,
  finished_at        TEXT,
  duration_ms        INTEGER,
  input_tokens       INTEGER NOT NULL DEFAULT 0,
  output_tokens      INTEGER NOT NULL DEFAULT 0,
  cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
  cache_write_tokens INTEGER NOT NULL DEFAULT 0,
  system_prompt      TEXT NOT NULL DEFAULT '',
  user_prompt        TEXT NOT NULL DEFAULT '',
  output             TEXT,
  error              TEXT
);
CREATE INDEX model_calls_id_idx ON model_calls(id DESC);
CREATE INDEX model_calls_state_idx ON model_calls(state, id DESC);

CREATE TABLE model_usage (
  day           TEXT PRIMARY KEY,
  calls         INTEGER NOT NULL DEFAULT 0,
  input_tokens  INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE settings (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  document   TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE resume_folder (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  path       TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE resume_profiles (
  code       TEXT NOT NULL,
  hash       TEXT NOT NULL,
  profile    TEXT NOT NULL,
  indexed_at TEXT NOT NULL,
  PRIMARY KEY (code, hash)
);

CREATE TABLE candidate_profile (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  profile    TEXT NOT NULL,
  source_key TEXT NOT NULL,
  model      TEXT NOT NULL DEFAULT '',
  indexed_at TEXT NOT NULL
);

CREATE TABLE plugin (
  id           INTEGER PRIMARY KEY CHECK (id = 1),
  extension_id TEXT NOT NULL DEFAULT '',
  version      TEXT NOT NULL DEFAULT '',
  capabilities TEXT NOT NULL DEFAULT '{}',
  first_seen   TEXT NOT NULL,
  last_seen    TEXT NOT NULL,
  hellos       INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE jobs (
  id                TEXT PRIMARY KEY,
  title             TEXT NOT NULL DEFAULT '',
  company           TEXT NOT NULL DEFAULT '',
  location          TEXT NOT NULL DEFAULT '',
  url               TEXT NOT NULL DEFAULT '',
  company_key       TEXT NOT NULL DEFAULT '',
  dupe_key          TEXT NOT NULL DEFAULT '',
  dupe_of           TEXT,
  description       TEXT,
  description_state TEXT NOT NULL DEFAULT 'pending',
  described_at      TEXT,
  listed_at         INTEGER,
  easy_apply        BOOLEAN NOT NULL DEFAULT FALSE,
  status            TEXT NOT NULL DEFAULT 'new',
  stage             TEXT NOT NULL DEFAULT 'none',
  score             INTEGER,
  triage_reason     TEXT,
  verdict_reason    TEXT,
  seniority         TEXT,
  workplace         TEXT,
  contract_type     TEXT,
  posting_lang      TEXT,
  agency            BOOLEAN,
  pay_amount        INTEGER,
  pay_currency      TEXT,
  pay_period        TEXT,
  resume_code       TEXT,
  resume_lang       TEXT,
  resume_fit        TEXT,
  tailored_needed   BOOLEAN,
  tailored_focus    TEXT,
  screen_key        TEXT,
  outcome           TEXT,
  outcome_note      TEXT,
  outcome_at        TEXT,
  cycle_id          INTEGER,
  query_label       TEXT,
  created_at        TEXT NOT NULL,
  updated_at        TEXT NOT NULL
);
CREATE INDEX jobs_status_idx ON jobs(status);
CREATE INDEX jobs_dupe_key_idx ON jobs(dupe_key);
CREATE INDEX jobs_listed_at_idx ON jobs(listed_at DESC);
CREATE INDEX jobs_created_at_idx ON jobs(created_at DESC);
CREATE INDEX jobs_screen_key_idx ON jobs(screen_key, status);
CREATE INDEX jobs_outcome_at_idx ON jobs(outcome_at DESC);

CREATE TABLE notes (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  scope         TEXT NOT NULL DEFAULT 'planning',
  body          TEXT NOT NULL,
  evidence      TEXT NOT NULL DEFAULT '',
  priority      TEXT NOT NULL DEFAULT 'normal',
  created_cycle INTEGER,
  updated_cycle INTEGER,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);
CREATE INDEX notes_scope_idx ON notes(scope, id DESC);

CREATE TABLE reflection (
  id       INTEGER PRIMARY KEY CHECK (id = 1),
  ran_at   TEXT NOT NULL,
  outcomes INTEGER NOT NULL DEFAULT 0,
  written  INTEGER NOT NULL DEFAULT 0,
  retired  INTEGER NOT NULL DEFAULT 0,
  model    TEXT NOT NULL DEFAULT ''
);

CREATE TABLE cycles (
  id                 INTEGER PRIMARY KEY AUTOINCREMENT,
  started_at         TEXT NOT NULL,
  finished_at        TEXT,
  status             TEXT NOT NULL DEFAULT 'open',
  rationale          TEXT NOT NULL DEFAULT '',
  screen_budget      INTEGER NOT NULL DEFAULT 0,
  received           INTEGER NOT NULL DEFAULT 0,
  inserted           INTEGER NOT NULL DEFAULT 0,
  duplicates         INTEGER NOT NULL DEFAULT 0,
  widening_steps     INTEGER NOT NULL DEFAULT 0,
  model_calls        INTEGER NOT NULL DEFAULT 0,
  input_tokens       INTEGER NOT NULL DEFAULT 0,
  output_tokens      INTEGER NOT NULL DEFAULT 0,
  termination_reason TEXT,
  error              TEXT
);
CREATE INDEX cycles_status_idx ON cycles(status, id DESC);

CREATE TABLE cycle_queries (
  cycle_id    INTEGER NOT NULL,
  position    INTEGER NOT NULL DEFAULT 0,
  label       TEXT NOT NULL,
  query       TEXT NOT NULL DEFAULT '{}',
  widened     BOOLEAN NOT NULL DEFAULT FALSE,
  pages       INTEGER NOT NULL DEFAULT 0,
  received    INTEGER NOT NULL DEFAULT 0,
  inserted    INTEGER NOT NULL DEFAULT 0,
  duplicates  INTEGER NOT NULL DEFAULT 0,
  skipped     BOOLEAN NOT NULL DEFAULT FALSE,
  skip_reason TEXT,
  PRIMARY KEY (cycle_id, label)
);

CREATE TABLE places (
  place_key     TEXT PRIMARY KEY,
  name          TEXT NOT NULL,
  asked         TEXT NOT NULL DEFAULT '',
  ring          TEXT NOT NULL DEFAULT '',
  location_id   TEXT NOT NULL,
  source        TEXT NOT NULL DEFAULT '',
  hits          INTEGER NOT NULL DEFAULT 1,
  first_seen_at TEXT NOT NULL,
  last_seen_at  TEXT NOT NULL
);
CREATE INDEX places_last_seen_at_idx ON places(last_seen_at DESC);
