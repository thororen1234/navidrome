-- +goose Up
CREATE TABLE download (
  id              TEXT PRIMARY KEY,
  tool            TEXT NOT NULL,
  source_url      TEXT NOT NULL DEFAULT '',
  tidal_id        TEXT NOT NULL DEFAULT '',
  tidal_kind      TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'queued',
  progress        REAL NOT NULL DEFAULT 0,
  status_message  TEXT NOT NULL DEFAULT '',
  error           TEXT NOT NULL DEFAULT '',
  library_id      INTEGER NOT NULL,
  target_path     TEXT NOT NULL DEFAULT '',
  requested_by    TEXT NOT NULL,
  attempts        INTEGER NOT NULL DEFAULT 0,
  created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  started_at      TIMESTAMP,
  completed_at    TIMESTAMP
);
-- Ordered to match DequeueBatch (status='queued', created_at ASC).
CREATE INDEX ix_download_status ON download(status, created_at);
CREATE INDEX ix_download_requested_by ON download(requested_by);

-- +goose Down
DROP TABLE download;
