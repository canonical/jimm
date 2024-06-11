-- 1_9.sql is a migration that deletes soft-deleted controllers and
-- drops the deleted_at column from the controllers table.
DELETE FROM controllers WHERE deleted_at IS NOT null;
ALTER TABLE controllers DROP COLUMN deleted_at;

UPDATE versions SET major=1, minor=9 WHERE component='jimmdb';
