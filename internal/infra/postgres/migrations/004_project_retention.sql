-- +goose Up
-- 004_project_retention.sql (PostgreSQL)
-- Adds optional retention window to projects. NULL = permanent.
-- INTEGER (PG) covers the same value range as SQLite's INTEGER
-- affinity here; no CHECK constraint — the REST surface clamps
-- to a sensible enum (1, 7, 30, 90, 365, NULL) at the handler.
ALTER TABLE projects ADD COLUMN retention_days INTEGER;
