-- alters the foreign key relationship `cloud_credentials.cloud_name -> clouds.name` to a cascade on-delete.

ALTER TABLE cloud_credentials
   DROP CONSTRAINT cloud_credentials_cloud_name_fkey,
   ADD CONSTRAINT cloud_credentials_cloud_name_fkey
      FOREIGN KEY (cloud_name)
      REFERENCES clouds(name)
      ON DELETE CASCADE;
