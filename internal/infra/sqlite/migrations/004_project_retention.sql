-- +goose Up
-- 004_project_retention.sql (SQLite)
-- Adds optional retention window to projects. NULL = permanent
-- (the default for existing rows). No purge daemon ships in this
-- release; the column records operator intent only.
ALTER TABLE projects ADD COLUMN retention_days INTEGER;
