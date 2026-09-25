# Notes

Migrations are applied using [golang-migrate](https://github.com/golang-migrate/migrate).
Previously migrations were applied using a home-grown solution and a `versions` table. 
The switch to golang-migrate was done to simplify our code.

To cater for existing deployments, we handle the case that the `versions` table still
exists and "force" the new migration tool to align with the old.

No "down" migrations are used currently. We aim to work with the philosophy that application
changes should be done such that we deprecate the use of any tables/columns, deploy these changes
and then later create a migration to make permanent changes to the DB. Ideally always moving
migrations forwards and never backwards.

By default, golang-migrate does not run migrations in a transactions.
**But**, the [postgres](https://github.com/golang-migrate/migrate/blob/master/database/postgres/README.md#multi-statement-mode) driver has slightly unique behavior - "running multiple SQL statements in one Exec executes them inside a transaction".
So each migration file is in fact run in a transaction when using PostgreSQL. To be more explicit, 
one can wrap the migration file with BEGIN/COMMIT instructions.

## Migration numbering

Numbers 1-999 are for the `v3` branch. Numbers 1000+ are for `feature/juju-4`.

- New v3 migration: next number below 1000 (e.g. `040_...`)
- New v4 migration: next number from 1000 (e.g. `1001_...`)
- Merging v3 into v4: keep v3's numbers as-is, v4's stay at 1000+. Never renumber.

Renumbering breaks in-place upgrades: a database at version N re-runs already-applied SQL or skips new migrations. Gaps in the sequence are fine.

### After v4 is released

If v3 receives a new migration after v4 has been released, copy it into the v4 migration range with a new number above 1000, clearly stating in a document which migration file you are copying. The v4 copy must be idempotent: it must succeed whether the original v3 migration has already run or not. This allows a v4 database already past version 1000 to receive the schema change, while allowing a database upgraded from the corresponding v3 release to run both migrations safely.
