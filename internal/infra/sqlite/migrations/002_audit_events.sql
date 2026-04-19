-- +goose Up

CREATE TABLE audit_events (
    id            TEXT PRIMARY KEY,
    occurred_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    event_type    TEXT NOT NULL
        CHECK (event_type IN (
            'agent_claimed',
            'agent_released',
            'lease_renewed',
            'lease_expired_by_sweeper'
        )),
    agent_id      TEXT NOT NULL,
    agent_name    TEXT NOT NULL DEFAULT '',
    workflow_id   TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT '',
    project       TEXT NOT NULL DEFAULT '',
    lease_expires_at DATETIME,
    payload       TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX audit_events_workflow_idx ON audit_events(workflow_id, occurred_at);
CREATE INDEX audit_events_agent_idx    ON audit_events(agent_id, occurred_at);
CREATE INDEX audit_events_type_idx     ON audit_events(event_type, occurred_at);

-- +goose Down

DROP INDEX audit_events_type_idx;
DROP INDEX audit_events_agent_idx;
DROP INDEX audit_events_workflow_idx;
DROP TABLE audit_events;
