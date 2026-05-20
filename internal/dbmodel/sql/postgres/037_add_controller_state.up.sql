-- Add a state field to the controllers table with a 
-- default value of 'active' that cannot be null.

ALTER TABLE controllers
    ADD COLUMN IF NOT EXISTS state TEXT;

UPDATE controllers
SET state = 'active'
WHERE state IS NULL OR state = '';

ALTER TABLE controllers
    ALTER COLUMN state SET DEFAULT 'active';

ALTER TABLE controllers
    ALTER COLUMN state SET NOT NULL;
