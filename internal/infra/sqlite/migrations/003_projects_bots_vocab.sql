-- +goose Up

CREATE TABLE vocabularies (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    description   TEXT NOT NULL DEFAULT '',
    release_event TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE vocabulary_events (
    id               TEXT PRIMARY KEY,
    vocabulary_id    TEXT NOT NULL REFERENCES vocabularies(id) ON DELETE CASCADE,
    event_type       TEXT NOT NULL,
    message_template TEXT NOT NULL,
    metadata_keys    TEXT NOT NULL DEFAULT '[]',
    UNIQUE (vocabulary_id, event_type)
);

CREATE TABLE projects (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    description   TEXT NOT NULL DEFAULT '',
    template_id   TEXT NOT NULL DEFAULT '',
    channel_name  TEXT NOT NULL,
    vocabulary_id TEXT NOT NULL REFERENCES vocabularies(id),
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE bots (
    id                     TEXT PRIMARY KEY,
    project_id             TEXT NOT NULL UNIQUE
        REFERENCES projects(id) ON DELETE CASCADE,
    passport_api_key_hash  TEXT NOT NULL,
    passport_api_key_id    TEXT NOT NULL,
    hive_role_assignments  TEXT NOT NULL DEFAULT '[]',
    created_at             DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at             DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX audit_events_project_idx ON audit_events(project, occurred_at);
CREATE INDEX work_items_assigned_agent_idx ON work_items(assigned_agent_id);

-- +goose Down

DROP INDEX work_items_assigned_agent_idx;
DROP INDEX audit_events_project_idx;
DROP TABLE bots;
DROP TABLE projects;
DROP TABLE vocabulary_events;
DROP TABLE vocabularies;
