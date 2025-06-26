-- adds migration_mode column to model table.
-- sets migration_mode to '' for all existing models.

CREATE TYPE migration_mode_type AS ENUM ('', 'exporting', 'importing');

ALTER TABLE models ADD COLUMN IF NOT EXISTS migration_mode migration_mode_type NOT NULL DEFAULT '';
