-- Delete soft-deleted cloud regions before removing soft-delete support.
-- Associated controller-priority rows are deleted by the foreign-key cascade.

DELETE FROM cloud_regions WHERE deleted_at IS NOT NULL;
DROP INDEX IF EXISTS idx_cloud_regions_deleted_at;
ALTER TABLE cloud_regions DROP COLUMN deleted_at;