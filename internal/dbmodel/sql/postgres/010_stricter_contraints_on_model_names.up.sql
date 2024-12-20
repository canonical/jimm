-- enforces a stricter uniqueness constraint on model names.

ALTER TABLE models DROP CONSTRAINT models_controller_id_owner_identity_name_name_key;
ALTER TABLE models ADD CONSTRAINT unique_model_names UNIQUE(owner_identity_name, name);
