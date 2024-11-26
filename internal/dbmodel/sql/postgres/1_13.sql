-- 1_13.sql is a migration that introduces the concept of roles to JIMM.
CREATE TABLE IF NOT EXISTS roles (
   id BIGSERIAL PRIMARY KEY,
   created_at TIMESTAMP WITH TIME ZONE,
   updated_at TIMESTAMP WITH TIME ZONE,
   name TEXT NOT NULL UNIQUE,
   uuid TEXT NOT NULL UNIQUE
);

UPDATE versions SET major=1, minor=13 WHERE component='jimmdb';
