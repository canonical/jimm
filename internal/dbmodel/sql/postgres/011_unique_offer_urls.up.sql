-- enforces uniqueness on URLs in application offers.

ALTER TABLE application_offers ADD UNIQUE (url);
