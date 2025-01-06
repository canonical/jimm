-- deletes soft-deleted controllers and drops the
-- deleted_at column from the controllers table.

DELETE FROM controllers WHERE deleted_at IS NOT null;
ALTER TABLE controllers DROP COLUMN deleted_at;
