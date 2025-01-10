-- Add the ability to store users' SSH public keys

CREATE TABLE ssh_keys (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE,
    public_key BYTEA NOT NULL,
    key_comment VARCHAR(255),
    identity_name TEXT NOT NULL,
    FOREIGN KEY (identity_name) REFERENCES identities(name),
    CONSTRAINT unique_identity_ssh_key UNIQUE(identity_name, public_key)
);

CREATE INDEX idx_ssh_keys_user_id ON ssh_keys (identity_name);
