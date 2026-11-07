CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);


-- ---------- Pocket Translate: everything below is per person ----------

-- One translation: what was put in, what came out, how. History and the
-- phrasebook are the same rows: a starred row is in the phrasebook, under
-- its collection ("Travel", "Work"). kind says where it came from: typed
-- text, a photo, speech, or another app through a Pocket Core link (source
-- is then that app's slug, and only that app may change the row).
CREATE TABLE translations (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind          TEXT NOT NULL DEFAULT 'text',          -- text | photo | speech | app | import
  source        TEXT NOT NULL DEFAULT '',              -- '' = this app; else the linked app's slug
  source_lang   TEXT NOT NULL DEFAULT '',              -- what was asked ('auto' or a code)
  detected_lang TEXT NOT NULL DEFAULT '',              -- what it turned out to be
  target_lang   TEXT NOT NULL,
  source_text   TEXT NOT NULL,
  result_text   TEXT NOT NULL DEFAULT '',
  engine        TEXT NOT NULL DEFAULT '',              -- ai | argos | nllb | import
  model         TEXT NOT NULL DEFAULT '',
  tone          TEXT NOT NULL DEFAULT '',              -- '' | formal | casual
  starred       INTEGER NOT NULL DEFAULT 0,
  collection    TEXT NOT NULL DEFAULT '',
  note          TEXT NOT NULL DEFAULT '',
  learn_ref     TEXT NOT NULL DEFAULT '',              -- the Pocket Learn card it became
  external_id   TEXT NOT NULL DEFAULT '',              -- an import's or a linked app's own id
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX translations_user ON translations(user_id, created_at DESC);
CREATE INDEX translations_starred ON translations(user_id, starred, collection);
CREATE UNIQUE INDEX translations_external ON translations(user_id, source, external_id) WHERE external_id <> '';

-- A person's own words: this term is always translated as that.
CREATE TABLE glossary (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  source_lang  TEXT NOT NULL,
  target_lang  TEXT NOT NULL,
  term         TEXT NOT NULL,
  translation  TEXT NOT NULL,
  note         TEXT NOT NULL DEFAULT '',
  match_case   INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX glossary_term ON glossary(user_id, source_lang, target_lang, term COLLATE NOCASE);

-- A document being translated, and when done its translated copy. The
-- files themselves are in document_files so a backup has everything.
CREATE TABLE documents (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  format       TEXT NOT NULL,                          -- docx | pptx | pdf | txt | md | srt | vtt | html
  mime         TEXT NOT NULL DEFAULT '',
  size         INTEGER NOT NULL DEFAULT 0,
  source_lang  TEXT NOT NULL DEFAULT 'auto',
  detected_lang TEXT NOT NULL DEFAULT '',
  target_lang  TEXT NOT NULL,
  engine       TEXT NOT NULL DEFAULT '',               -- what the person chose: auto | ai | offline
  used         TEXT NOT NULL DEFAULT '',               -- what translated it
  status       TEXT NOT NULL DEFAULT 'queued',         -- queued | working | done | failed
  done         INTEGER NOT NULL DEFAULT 0,             -- pieces translated
  total        INTEGER NOT NULL DEFAULT 0,
  error        TEXT NOT NULL DEFAULT '',
  result_name  TEXT NOT NULL DEFAULT '',
  origin       TEXT NOT NULL DEFAULT '',               -- upload | files:<path> | documents:<id>
  saved        TEXT NOT NULL DEFAULT '[]',             -- where the result was saved: [{app, title, url}]
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  finished_at  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX documents_user ON documents(user_id, created_at DESC);
CREATE INDEX documents_status ON documents(status);

CREATE TABLE document_files (
  document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  which       TEXT NOT NULL,                           -- original | result
  data        BLOB NOT NULL,
  PRIMARY KEY (document_id, which)
);

-- A spoken (or typed) conversation between two languages, turn by turn.
CREATE TABLE conversations (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title       TEXT NOT NULL DEFAULT '',
  lang_a      TEXT NOT NULL,                           -- my side
  lang_b      TEXT NOT NULL,                           -- the other person's side
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX conversations_user ON conversations(user_id, updated_at DESC);

CREATE TABLE turns (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  side            TEXT NOT NULL,                       -- a | b
  spoken          INTEGER NOT NULL DEFAULT 0,          -- said into the microphone (else typed)
  source_lang     TEXT NOT NULL,
  target_lang     TEXT NOT NULL,
  source_text     TEXT NOT NULL,
  result_text     TEXT NOT NULL DEFAULT '',
  engine          TEXT NOT NULL DEFAULT '',
  created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX turns_conv ON turns(conversation_id, id);
CREATE INDEX turns_user ON turns(user_id, created_at);
