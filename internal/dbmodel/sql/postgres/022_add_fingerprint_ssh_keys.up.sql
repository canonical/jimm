-- store public keys fingerprints

ALTER TABLE ssh_keys ADD COLUMN md5_fingerprint VARCHAR(50) NOT NULL;
