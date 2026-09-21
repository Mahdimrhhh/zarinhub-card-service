CREATE TABLE IF NOT EXISTS audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    duration_ms INTEGER NOT NULL,
    client_ip TEXT NOT NULL,
    card_masked TEXT NOT NULL,
    status TEXT NOT NULL,
    external_http_status INTEGER,
    response_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_audit_events_request_id ON audit_events(request_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_created_at ON audit_events(created_at);
