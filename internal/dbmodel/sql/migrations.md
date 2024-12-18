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
