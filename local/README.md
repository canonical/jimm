# Local Development & Testing

This doc is intended to help those new to JIMM get up and running
with the local Q/A environment. This environment is additionally
used for integration testing within the JIMM test suite.

# Starting the environment

1. Ensure you have `make` installed `sudo apt install make`
2. Check for system dependencies with `make sys-deps` this will inform you of any missing dependencies and how to install them.
3. Set up necessary prerequisites with `make dev-env-setup`
4. Start the dev env `make dev-env`, going forward you can skip steps 1-3.
5. To teardown the dev env `make dev-env-cleanup`

The service is started using Docker Compose, the following services should be started:
- JIMM (only started in the dev profile)
- Traefik (only started in the dev profile)
- Vault
- Postgres
- OpenFGA

Some notes on the setup:
- Local images are created in the repo's `/local/<service>` folder where any init scripts are defined for each service using the service's upstream docker image.
- The docker compose has a base at `docker-compose.common.yaml` for common elements to reduce duplication.
- The compose has 2 additional profiles (dev and test). 
  - Starting the compose with no profile will spin up the necessary components for testing.
  - The dev profile will start JIMM in a container using [air](https://github.com/air-verse/air), a tool for auto-reloading Go code when the source changes.
  - The test profile will start JIMM by pulling a version of the JIMM image from a container registry, useful in integration tests.

> Any changes made inside the repo will automatically restart the JIMM server via a volume mount + air. So there's no need to re-run the compose continuously.

# Q/A Using jimmctl

## Prerequisites

// TODO(): Ipv6 network on the Juju container don't work with JIMM. Figure out how to disable these at the container level so that the controller.yaml file doesn't present ipv6 at all. For now one can remove this by hand.

1. The following commands might need to be run to work around an [LXC networking
   issue](https://github.com/docker/for-linux/issues/103#issuecomment-383607773):
   `sudo iptables -F FORWARD && sudo iptables -P FORWARD ACCEPT`.
2. Install Juju: `sudo snap install juju --channel=3.5/stable` (minimum required Juju version is `3.5`).
3. Install JQ: `sudo snap install jq`.

## All-In-One commands

We have two all-in-one commands, namely:
- LXD: run `make qa-lxd`
- K8s: run `make qa-microk8s`

These scripts respectively spin up jimm in compose, setup controllers in the targeted environment
and handle connectivity. Finally, adding a test model to Q/A against.

Please ensure you've run "make dev-env-setup" first though.

## Manual

### Controller set up

Note that you can export an environment variable `CONTROLLER_NAME` and re-run steps 3. and 4. below to create multiple Juju
controllers that will be controlled by JIMM.

1. `juju unregister jimm-dev`                     - Unregister any other local JIMM you have.
2. `juju login jimm.localhost -c jimm-dev`        - Login to local JIMM with username "jimm-test" password "password".
3. `./local/jimm/setup-controller.sh`             - Performs controller setup.
   > If LXD is not initialized, run `lxd init --auto` first.
4. `./local/jimm/add-controller.sh`               - A local script to do many of the manual steps for us. See script for more details.
5. `juju add-model test`                          - Adds a model to qa-lxd via JIMM.

# Helpful tidbits!

> Note: For any secure step to work, ensure you've run the local traefik certs script!

- To access vault UI, the URL is: `http://localhost:8200/ui` and the root key is `token`.
- The WS API for JIMM Controller is under: `ws://localhost:17070` (http direct) and `wss://jimm.localhost` for secure.
- You can verify local deployment with: `curl http://localhost:17070/debug/status` and `curl https://jimm.localhost/debug/status`
- Traefik is available on `http://localhost:8089`.
