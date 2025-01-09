-- deletes secrets that were directly stored in the database.

ALTER TABLE cloud_credentials DROP COLUMN IF EXISTS attributes_in_vault;
ALTER TABLE cloud_credentials DROP COLUMN IF EXISTS attributes;
ALTER TABLE controllers DROP COLUMN IF EXISTS admin_identity_name;
ALTER TABLE controllers DROP COLUMN IF EXISTS admin_password;
