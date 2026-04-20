-- +goose Up
-- 005_workflow_instance_project_fk.sql (SQLite)
-- Adds nullable project_id FK to workflow_instances. SQLite's
-- ALTER TABLE ADD COLUMN cannot inline the REFERENCES clause
-- (limitation of SQLite ALTER), so the column is plain TEXT and
-- the FK relationship is documented + enforced at the handler /
-- store layer. Pre-existing rows get NULL. An index on the new
-- column keeps the per-project list endpoint O(log n).
ALTER TABLE workflow_instances ADD COLUMN project_id TEXT;
CREATE INDEX IF NOT EXISTS idx_workflow_instances_project_id
    ON workflow_instances(project_id);
