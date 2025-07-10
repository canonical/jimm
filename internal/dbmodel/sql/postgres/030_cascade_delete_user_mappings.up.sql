-- Add foreign key constraint between user mappings and 
-- models with ON DELETE CASCADE to ensure user mappings
-- are deleted when the corresponding model is removed.

-- Add a foreign key constraint with ON DELETE CASCADE
ALTER TABLE user_mappings
    ADD CONSTRAINT fk_user_mappings_model_uuid
    FOREIGN KEY (model_uuid)
    REFERENCES models(uuid)
    ON DELETE CASCADE;
