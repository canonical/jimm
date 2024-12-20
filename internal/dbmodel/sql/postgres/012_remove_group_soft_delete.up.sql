-- deletes soft-deleted groups and drops the deleted_at column from the groups table.

DELETE FROM groups WHERE deleted_at IS NOT null;
ALTER TABLE groups DROP COLUMN deleted_at;
