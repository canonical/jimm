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

   or to run tests against multiple backing controllers you can bootstrap another controller:
   ```bash
   JWKS_DNS=jwks.localhost CONTROLLER_NAME=test-e2e-2 ./local/jimm/setup-controller.sh
   ```
   and generate the file with multiple controllers.
   ```bash
   CONTROLLERS=test-e2e,test-e2e2  make generate-test-env-multiple
   ```


## Setup microk8s cloud

Our tests run using an lxd backing controller against the lxd cloud, but there are suites requiring a microk8s cloud to test specific
JAAS functionalities.

After setting up microk8s we add the k8s cloud to the client:
`juju add-k8s microk8s-cp --client`

Then, to make sure the lxd backing controller is able to talk with the k8s API server we need to make
sure that the k8s API endpoint inside the cloud config is reachable from within the k8s cluster because Juju workers will use it.
The easiest way to do so is to use lxc devices.
```
lxc config device add <lxc_name_running_juju_controller> k8sproxy proxy \
         listen=tcp:127.0.0.1:16443 \
         connect=tcp:<your_host_machine_ip>:16443 \
         bind=container
```

In this way when the lxc controller tries to connect to the k8s API server for microk8s it will use `127.0.0.1`, which is proxied to the host machine.

Then, configure the environvent variable to use the microk8s cloud to run the specific suite:
- `JIMM_MICROK8S_TEST_CLOUD_NAME = microk8s-cp`

## VS Code Configuration

Add the following environment variables to your `go.testEnvVars` in `.vscode/settings.json`:

```json
{
    "go.testEnvVars": {
        "RUN_E2E_TESTS": "1",
        "XDG_DATA_HOME": "~/.local/share",
        "JIMM_BACKING_CONTROLLER_CONFIG": "/home/<user>/jaas/jimm/controllers.yaml",
        "JIMM_MICROK8S_TEST_CLOUD_NAME": "microk8s-cp"
    }
}
```

> **Notes:**
> - Set `RUN_E2E_TESTS=1` to enable the e2e tests. Without this, the tests will be skipped.
> - Use the full absolute path for `JIMM_BACKING_CONTROLLER_CONFIG`. The `${workspaceFolder}` variable is not expanded by the VS Code Go extension.
> - If VS Code is installed as a snap, set `XDG_DATA_HOME` to `~/.local/share`. This ensures the tests can access the Juju client store at the correct location to retrieve cloud credentials.
