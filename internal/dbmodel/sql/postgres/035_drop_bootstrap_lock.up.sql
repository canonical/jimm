--- Drop the bootstrap lock table.
--- With the move to River queue, concurrency control for bootstrapping is now handled
--- by River, so this table is no longer needed.

DROP TABLE IF EXISTS bootstrap_locks;