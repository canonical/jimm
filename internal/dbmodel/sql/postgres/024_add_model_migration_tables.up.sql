-- Add tables to handle model migrations
-- The incoming_model_migration table contains information
-- about models migrating into JIMM and their target controller.
-- The UserMapping table is used to map local users
-- from migrated models to their external counterparts.

CREATE TABLE incoming_model_migrations (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    model_uuid TEXT NOT NULL UNIQUE,
    target_controller_id INTEGER REFERENCES controllers (id),
    user_mapping JSONB NOT NULL
);

CREATE TABLE user_mappings (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE,
    model_uuid TEXT NOT NULL,
    local_user TEXT NOT NULL,
    external_user TEXT NOT NULL REFERENCES identities(name),
    CONSTRAINT unique_user_mappings_key UNIQUE(model_uuid, local_user)
);

CREATE INDEX idx_model_user_mappings_model_uuid ON user_mappings (model_uuid);
