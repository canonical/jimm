-- create a table to hold the bootstrap lock
-- the name is plural because GORM requires it, even if we will only have one row.
-- create a row with id 0 to hold the lock.

-- The reasons for using this simpler approach instead of advisory locks are:
-- - this approach is straightforward and easy to understand.
-- - advisory locks are tight to the session, this means that you need to acquire and
--   release the lock in the same session.
-- - the advisory lock can be acquired multiple times in the same session, and this is an issue because 
--   we want to ensure that the lock is only held once independently of the connection used.
-- - advisory locks are difficult to use with connection pooling or when using pg-bouncer.

CREATE TABLE IF NOT EXISTS bootstrap_locks (
    id BIGSERIAL PRIMARY KEY,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    locked BOOLEAN NOT NULL
);

INSERT INTO bootstrap_locks (expires_at, locked) VALUES (NOW(), FALSE);
