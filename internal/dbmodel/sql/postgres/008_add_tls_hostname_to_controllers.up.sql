-- adds a tls_hostname column to the controller table.

ALTER TABLE controllers ADD COLUMN tls_hostname TEXT;
