---
name: spin-up-test-env
description: >-
  Reuse or start JIMM's local Docker Compose test services for tests that need
  PostgreSQL, Vault, OpenFGA, or Keycloak. Use for service-backed integration
  tests, end-to-end tests under testing/ or tests/, or when a test fails because
  one of these services is unavailable.
---

# Start the JIMM test environment

Skip this setup for unit tests that do not need external services.

1. Reuse the backing services. Keep a readiness record in the session database
   (`compose_readiness`), keyed by repository root, Compose project, and service.
   After a successful startup or health check, record which services are ready. If the required
   services are already recorded as ready for this session, go straight to the
   tests; do not run Compose again unless a service error points to a problem.
   If there is no record, first check `docker compose ps -a` from the repository
   root. Record and reuse healthy services. Start the stack only if a required
   service is missing, stopped, or unhealthy:

   ```bash
   docker compose up -d --wait
   ```

   Record readiness only after Compose reports success for the required services.
   Use `make sys-deps` to check for missing tools. `make test-env` also runs those
   checks, but force-recreates the containers; use it only when it is safe to
   interrupt the existing stack.

2. Check any additional prerequisites. The default Compose stack does not start JIMM or bootstrap a Juju controller.
   - For Go end-to-end tests, follow [testing/README.md](../../../testing/README.md) to configure a backing controller and enable the tests with `RUN_E2E_TESTS=1`.
   - For shell tests, check the selected script's prerequisites. See [tests/README.md](../../../tests/README.md) for how to run the tests and the “Q/A Setup” section of the [local development guide](../../../local/README.md) for JIMM and Juju setup.

3. Run the narrowest relevant test command once the services and any additional prerequisites are ready.

## Troubleshooting and cleanup

- If a test reports a service connection or readiness error, invalidate the
  affected readiness record and inspect `docker compose ps -a` and the service
  logs. Run startup again only if a required service is not ready. If startup
  fails and the service remains unavailable, report the blocker. An ordinary
  test failure is not a reason to run Compose again.
- Leave the stack running unless the user requests cleanup. Get explicit confirmation before running `make test-env-cleanup` or `docker compose down -v --remove-orphans`: both delete Compose-managed volumes and their data.