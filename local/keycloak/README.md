# Keycloak

This directory contains the local Keycloak setup used by JIMM development. The realm import is intentionally minimal: it only seeds the `jimm` realm with the clients and users that the local stack and tests rely on.

The Keycloak admin account is not part of the realm import. It is created by the container on startup from the credentials in `docker-compose.yaml`.

## Default accounts

| Purpose | Realm | Username | Email | Password |
| --- | --- | --- | --- | --- |
| Keycloak admin console | `master` | `jimm` | n/a | `jimm` |
| JIMM admin user | `jimm` | `jimm-test` | `jimm-test@canonical.com` | `password` |
| JIMM non-admin user | `jimm` | `jimm-user` | `jimm-user@canonical.com` | `password` |

`jimm-test` is treated as a JIMM admin because `docker-compose.common.yaml` sets `JIMM_ADMINS=jimm-test@canonical.com`.

There is also a test-only user, `jimm_test` / `password`, kept in the realm for unsafe-email login coverage in the e2e test suite.

## Log in as the Keycloak admin user

1. Start the local environment with `make dev-env` or `docker compose up -d keycloak`.
2. Open `http://keycloak.localhost:8082/admin/master/console/`.
3. Sign in with username `jimm` and password `jimm`.
4. After login, switch the realm selector from `master` to `jimm` if you want to inspect or manage the application realm.

## Log in as a regular JIMM user

Use the `jimm` realm accounts when you want to test the application itself instead of the Keycloak admin console:

- Use `jimm-test` / `password` for the default JIMM admin user.
- Use `jimm-user` / `password` for the default non-admin user.

## Create more users

Run `./local/keycloak/create-user.sh [username] [password] [email]`.

The script authenticates with the default Keycloak admin account and creates the user in the `jimm` realm.

