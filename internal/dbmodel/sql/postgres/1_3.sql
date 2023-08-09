-- 1_3.sql is a migration that alters the foreign key relationship `cloud_credentials.cloud_name -> clouds.name` to a cascade on-delete.

alter table cloud_credentials
drop constraint cloud_credentials_cloud_name_fkey,
add constraint cloud_credentials_cloud_name_fkey 
   foreign key (cloud_name)
   references clouds(name)
   on delete cascade;
ALTER TABLE

UPDATE versions SET major=1, minor=3 WHERE component='jimmdb';
