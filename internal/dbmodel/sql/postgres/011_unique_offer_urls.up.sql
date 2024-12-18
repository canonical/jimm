-- 1_11.sql is a migration that enforces uniqueness on URLs in application offers.

ALTER TABLE application_offers ADD UNIQUE (url);
