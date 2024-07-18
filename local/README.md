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
- Vault
- Postgres
- OpenFGA
- Traefik

> Any changes made inside the repo will automatically restart the JIMM server via a volume mount. So there's no need
to re-run the compose continuously, but note, if you do bring the compose down, remove the volumes otherwise
vault will not behave correctly, this can be done via `docker compose down -v`

Now please checkout the [Authentication Steps](#authentication-steps) to authenticate postman for local testing & Q/A.

# Q/A Using Postman
#### Setup
1. Run `make get-local-auth`
2. Head to postman and follow the instructions given by get-local-auth.
#### Facades in Postman
You will see JIMM's controller WS API broken up into separate WS requests.
This is intentional.
Inside of each WS request will be a set of `saved messages` (on the right-hand side), these are the calls to facades for the given API under that request.

The `request name` represents the literal WS endpoint, i.e., `API = /api`.

> Remember to run the `Login` message when spinning up a new WS connection, otherwise you will not be able to send subsequent calls to this WS.


# Q/A Using jimmctl

## Prerequisites

// TODO(): Ipv6 network on the Juju container don't work with JIMM. Figure out how to disable these at the container level so that the controller.yaml file doesn't present ipv6 at all. For now one can remove this by hand.

1. The following commands might need to be run to work around an [LXC networking
   issue](https://github.com/docker/for-linux/issues/103#issuecomment-383607773):
   `sudo iptables -F FORWARD && sudo iptables -P FORWARD ACCEPT`.
2. Install Juju: `sudo snap install juju --channel=3.5/stable` (minimum Juju version is `3.5`).
3. Install JQ: `sudo snap install jq`.

## All-In-One scripts
We have two all-in-one scripts, namely:
- qa-lxd.sh
- qa-microk8s.sh
These scripts respectively spin up jimm in compose, setup controllers in the targeted environment
and handle connectivity. Finally, adding a test model to Q/A against.

Please ensure you've run "make dev-env-setup" first though!

## Manual
### Controller set up

Note that you can export an environment variable `CONTROLLER_NAME` and re-run steps 3. and 4. below to create multiple Juju
controllers that will be controlled by JIMM.

1. `juju unregister jimm-dev`                     - Unregister any other local JIMM you have.
2. `juju login jimm.localhost -c jimm-dev`        - Login to local JIMM with username "jimm-test" password "password"
3. `./local/jimm/setup-controller.sh`             - Performs controller setup.
4. `./local/jimm/add-controller.sh`               - A local script to do many of the manual steps for us. See script for more details.
5. `juju add-model test`                          - Adds a model to qa-lxd via JIMM.

# Helpful tidbits!
> Note: For any secure step to work, ensure you've run the local traefik certs script!

- To access vault UI, the URL is: `http://localhost:8200/ui` and the root key is `token`.
- The WS API for JIMM Controller is under: `ws://localhost:17070` (http direct) and `wss://jimm.localhost` for secure.
- You can verify local deployment with: `curl http://localhost:17070/debug/status` and `curl https://jimm.localhost/debug/status`
- Traefik is available on `http://localhost:8089`.
