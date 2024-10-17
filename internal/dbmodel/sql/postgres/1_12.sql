-- 1_12.sql is a migration that deletes soft-deleted groups and
-- drops the deleted_at column from the groups table.
DELETE FROM groups WHERE deleted_at IS NOT null;
ALTER TABLE groups DROP COLUMN deleted_at;

UPDATE versions SET major=1, minor=12 WHERE component='jimmdb';
