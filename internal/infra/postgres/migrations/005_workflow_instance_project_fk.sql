-- +goose Up
-- 005_workflow_instance_project_fk.sql (PostgreSQL)
-- Adds nullable project_id FK to workflow_instances. PG supports
-- the inline REFERENCES clause (unlike SQLite's ALTER TABLE), so
-- the FK is enforced at the database layer. ON DELETE SET NULL
-- mirrors the "pre-existing rows have NULL project" semantics —
-- deleting a project does not cascade-delete its instances; the
-- instances become unbound and remain reachable via team_id.
ALTER TABLE workflow_instances
    ADD COLUMN project_id TEXT REFERENCES projects(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_workflow_instances_project_id
    ON workflow_instances(project_id);
