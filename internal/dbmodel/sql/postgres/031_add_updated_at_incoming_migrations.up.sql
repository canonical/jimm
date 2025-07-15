-- Add an updated_at column to the incoming_model_migrations table

ALTER TABLE incoming_model_migrations
    ADD COLUMN updated_at TIMESTAMP WITH TIME ZONE;
