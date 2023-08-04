-- 1_3.sql is a migration that alters the foreign key relationship `cloud_credentials.cloud_name -> clouds.name` to a cascade on-delete.
-- Followed official instructions under heading "Making Other Kinds Of Table Schema Changes" at:
--   - https://www.sqlite.org/lang_altertable.html
--   - http://web.archive.org/web/20230718062623/https://www.sqlite.org/lang_altertable.html

PRAGMA schema_version;
PRAGMA writable_schema=ON;
UPDATE sqlite_schema SET sql=REPLACE(sql,'cloud_name TEXT NOT NULL REFERENCES clouds (name)','cloud_name TEXT NOT NULL REFERENCES clouds (name) ON DELETE CASCADE') WHERE type='table' AND name='cloud_credentials';
PRAGMA writable_schema=OFF;
PRAGMA integrity_check;

UPDATE versions SET major=1, minor=3 WHERE component='jimmdb';
