-- Creates a trigger to prevent the controller being unregistered/deleted if a migration is active.
-- It checks for a "migration_active" bool, and if it's true, prevents deletes.
ALTER TABLE controllers ADD COLUMN migration_active boolean DEFAULT false;

CREATE OR REPLACE FUNCTION prevent_unregister_if_migration_active()
RETURNS trigger AS $$
BEGIN
  IF OLD.migration_active THEN
    RAISE EXCEPTION 'migration is in progress';
  END IF;
  RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER prevent_unregister_if_migration_active
BEFORE DELETE ON controllers
FOR EACH ROW
EXECUTE FUNCTION prevent_unregister_if_migration_active();