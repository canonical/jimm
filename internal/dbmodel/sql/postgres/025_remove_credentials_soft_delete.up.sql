-- deletes soft-deleted cloud_credentials and drops the
-- deleted_at column from the cloud_credentials table.

DELETE FROM cloud_credentials WHERE deleted_at IS NOT null;
ALTER TABLE cloud_credentials DROP COLUMN deleted_at;
