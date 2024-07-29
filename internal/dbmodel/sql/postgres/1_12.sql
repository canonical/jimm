
-- 1_12.sql adds the ability to store Juju users SSH public keys and enables the implementation
-- of adding users SSH keys to JIMM.

CREATE TABLE user_ssh_keys (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    identity_name VARCHAR(255),
    ssh_key TEXT NOT NULL,
    FOREIGN KEY (identity_name) REFERENCES identities(name)
);

UPDATE versions SET major=1, minor=12 WHERE component='jimmdb';
