-- Stores the cloud TLS verification setting returned by Juju.

ALTER TABLE clouds
    ADD COLUMN skip_tls_verify BOOLEAN NOT NULL DEFAULT FALSE;