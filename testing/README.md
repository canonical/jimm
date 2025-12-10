# E2E Testing Setup

This guide outlines the steps to set up and run the e2e test suite.

## Prerequisites

1. **Start the test environment**

   Start the dev environment to run these tests:

   ```bash
   make test-env
   ```

2. **Set up a controller with JWKS endpoint**

   Bootstrap a controller configured to use the JWKS endpoint:

   ```bash
   JWKS_DNS=jwks.localhost CONTROLLER_NAME=test-e2e ./local/jimm/setup-controller.sh
   ```

3. **Generate the controller config**

   Create the YAML file with the controller configuration used by the test suite to add the backing controller to JIMM:

   ```bash
   CONTROLLER=test-e2e make generate-test-env
   ```

## VS Code Configuration

Add the following environment variables to your `go.testEnvVars` in `.vscode/settings.json`:

```json
{
    "go.testEnvVars": {
        "RUN_E2E_TESTS": "1",
        "XDG_DATA_HOME": "~/.local/share",
        "JIMM_BACKING_CONTROLLER_CONFIG": "/home/<user>/jaas/jimm/controllers.yaml"
    }
}
```

> **Notes:**
> - Set `RUN_E2E_TESTS=1` to enable the e2e tests. Without this, the tests will be skipped.
> - Use the full absolute path for `JIMM_BACKING_CONTROLLER_CONFIG`. The `${workspaceFolder}` variable is not expanded by the VS Code Go extension.
> - If VS Code is installed as a snap, set `XDG_DATA_HOME` to `~/.local/share`. This ensures the tests can access the Juju client store at the correct location to retrieve cloud credentials.
