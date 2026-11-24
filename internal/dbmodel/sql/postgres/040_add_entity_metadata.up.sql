CREATE TABLE IF NOT EXISTS entity_metadata (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    entity_type TEXT NOT NULL,
    entity TEXT NOT NULL,
    model_uuid TEXT NOT NULL REFERENCES models (uuid) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    UNIQUE (model_uuid, entity_type, entity, key)
);
