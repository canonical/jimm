---
name: spin-up-test-env
description: >-
  Start JIMM's local Docker Compose test services before running tests that need
  PostgreSQL, Vault, OpenFGA, or Keycloak. Use for service-backed integration
  tests, end-to-end tests under testing/ or tests/, or when a test fails because
  one of these services is unavailable.
---

# Start the JIMM test environment

Skip this setup for unit tests that do not need external services.

1. From the repository root, start the backing services and wait for Compose to report success:

   ```bash
   docker compose up -d --wait
   ```

   This reuses existing containers when possible. Use `make sys-deps` to check for missing tools. `make test-env` also runs those checks, but force-recreates the containers; use it only when it is safe to interrupt the existing stack.

2. Check any additional prerequisites. The default Compose stack does not start JIMM or bootstrap a Juju controller.
   - For Go end-to-end tests, follow [testing/README.md](../../../testing/README.md) to configure a backing controller and enable the tests with `RUN_E2E_TESTS=1`.
   - For shell tests, check the selected script's prerequisites. See [tests/README.md](../../../tests/README.md) for how to run the tests and the “Q/A Setup” section of the [local development guide](../../../local/README.md) for JIMM and Juju setup.

3. Run the narrowest relevant test command once the services and any additional prerequisites are ready.

## Troubleshooting and cleanup

- If startup fails, inspect `docker compose ps -a` and the failing service's logs before retrying. If the service is still unavailable, report the blocker instead of running tests that depend on it.
- Leave the stack running unless the user requests cleanup. Get explicit confirmation before running `make test-env-cleanup` or `docker compose down -v --remove-orphans`: both delete Compose-managed volumes and their data.