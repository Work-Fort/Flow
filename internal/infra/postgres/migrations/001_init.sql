-- +goose Up

CREATE TABLE workflow_templates (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    version     INTEGER NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE steps (
    id           TEXT PRIMARY KEY,
    template_id  TEXT NOT NULL REFERENCES workflow_templates(id) ON DELETE CASCADE,
    key          TEXT NOT NULL,
    name         TEXT NOT NULL,
    type         TEXT NOT NULL CHECK (type IN ('task', 'gate')),
    position     INTEGER NOT NULL DEFAULT 0,
    approval_mode              TEXT CHECK (approval_mode IN ('any', 'unanimous')),
    approval_required          INTEGER,
    approval_approver_role_id  TEXT,
    approval_rejection_step_id TEXT,
    UNIQUE (template_id, key)
);

CREATE TABLE transitions (
    id               TEXT PRIMARY KEY,
    template_id      TEXT NOT NULL REFERENCES workflow_templates(id) ON DELETE CASCADE,
    key              TEXT NOT NULL,
    name             TEXT NOT NULL,
    from_step_id     TEXT NOT NULL REFERENCES steps(id),
    to_step_id       TEXT NOT NULL REFERENCES steps(id),
    guard            TEXT NOT NULL DEFAULT '',
    required_role_id TEXT NOT NULL DEFAULT '',
    UNIQUE (template_id, key)
);

CREATE TABLE role_mappings (
    id              TEXT PRIMARY KEY,
    template_id     TEXT NOT NULL REFERENCES workflow_templates(id) ON DELETE CASCADE,
    step_id         TEXT NOT NULL REFERENCES steps(id) ON DELETE CASCADE,
    role_id         TEXT NOT NULL,
    allowed_actions TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE integration_hooks (
    id            TEXT PRIMARY KEY,
    template_id   TEXT NOT NULL REFERENCES workflow_templates(id) ON DELETE CASCADE,
    transition_id TEXT NOT NULL REFERENCES transitions(id) ON DELETE CASCADE,
    event         TEXT NOT NULL DEFAULT 'on_transition',
    adapter_type  TEXT NOT NULL,
    action        TEXT NOT NULL,
    config        TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE workflow_instances (
    id               TEXT PRIMARY KEY,
    template_id      TEXT NOT NULL REFERENCES workflow_templates(id),
    template_version INTEGER NOT NULL,
    team_id          TEXT NOT NULL,
    name             TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'paused', 'completed', 'archived')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE integration_configs (
    id           TEXT PRIMARY KEY,
    instance_id  TEXT NOT NULL REFERENCES workflow_instances(id) ON DELETE CASCADE,
    adapter_type TEXT NOT NULL,
    config       TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE work_items (
    id                TEXT PRIMARY KEY,
    instance_id       TEXT NOT NULL REFERENCES workflow_instances(id),
    title             TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    current_step_id   TEXT NOT NULL REFERENCES steps(id),
    assigned_agent_id TEXT NOT NULL DEFAULT '',
    priority          TEXT NOT NULL DEFAULT 'normal'
        CHECK (priority IN ('critical', 'high', 'normal', 'low')),
    fields            TEXT NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE external_links (
    id           TEXT PRIMARY KEY,
    work_item_id TEXT NOT NULL REFERENCES work_items(id) ON DELETE CASCADE,
    service_type TEXT NOT NULL,
    adapter      TEXT NOT NULL,
    external_id  TEXT NOT NULL,
    url          TEXT NOT NULL DEFAULT ''
);

CREATE TABLE transition_history (
    id            TEXT PRIMARY KEY,
    work_item_id  TEXT NOT NULL REFERENCES work_items(id),
    from_step_id  TEXT NOT NULL REFERENCES steps(id),
    to_step_id    TEXT NOT NULL REFERENCES steps(id),
    transition_id TEXT NOT NULL REFERENCES transitions(id),
    triggered_by  TEXT NOT NULL,
    reason        TEXT NOT NULL DEFAULT '',
    timestamp     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE approvals (
    id           TEXT PRIMARY KEY,
    work_item_id TEXT NOT NULL REFERENCES work_items(id),
    step_id      TEXT NOT NULL REFERENCES steps(id),
    agent_id     TEXT NOT NULL,
    decision     TEXT NOT NULL CHECK (decision IN ('approved', 'rejected')),
    comment      TEXT NOT NULL DEFAULT '',
    timestamp    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down

DROP TABLE approvals;
DROP TABLE transition_history;
DROP TABLE external_links;
DROP TABLE work_items;
DROP TABLE integration_configs;
DROP TABLE workflow_instances;
DROP TABLE integration_hooks;
DROP TABLE role_mappings;
DROP TABLE transitions;
DROP TABLE steps;
DROP TABLE workflow_templates;
