-- add model association to user ssh keys
ALTER TABLE ssh_keys DROP CONSTRAINT unique_identity_ssh_key;

ALTER TABLE ssh_keys 
    ADD COLUMN model_uuid TEXT,
    ADD CONSTRAINT fk FOREIGN KEY(model_uuid) REFERENCES models(uuid) ON DELETE CASCADE;

ALTER TABLE ssh_keys ADD CONSTRAINT unique_identity_ssh_key UNIQUE(identity_name, public_key, model_uuid);
