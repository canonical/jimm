-- Add internal migration value to migration_mode_type enum.

ALTER TYPE migration_mode_type ADD VALUE 'migrating-internally';
