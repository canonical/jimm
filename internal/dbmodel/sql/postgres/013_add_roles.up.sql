-- introduces the concept of roles to JIMM.

CREATE TABLE IF NOT EXISTS roles (
   id BIGSERIAL PRIMARY KEY,
   created_at TIMESTAMP WITH TIME ZONE,
   updated_at TIMESTAMP WITH TIME ZONE,
   name TEXT NOT NULL UNIQUE,
   uuid TEXT NOT NULL UNIQUE
);
