# Local Development & Testing

This doc is intended to help those new to JIMM get up and running
with the local Q/A environment. This environment is additionally
used for integration testing within the JIMM test suite.

# Starting the environment
1. Run `make pull/candid` to get a local image of candid (this is subject to change!)
2. Run `./certs.sh` in local/traefik/certs, be sure to be in that directory though!
2. docker compose up

The services included are:
- JIMM
- Vault
- Postgres
- OpenFGA
- Traefik

> Any changes made inside the repo will automatically restart the JIMM server via a volume mount. So there's no need
to re-run the compose continuously, but note, if you do bring the compose down, remove the volumes otherwise
vault will not behave correctly, this can be done via `docker compose down -v`

If all was successful, you should seen an output similar to:
```
NAME                COMMAND                  SERVICE             STATUS                PORTS
candid              candid:latest       "/candid.sh"             candid              About a minute ago   Up About a minute (healthy)     ...
jimmy               cosmtrek/air        "/go/bin/air"            jimm                About a minute ago   Up 54 seconds (healthy)         ...
openfga             jimm-openfga        "/app/openfga run"       openfga             About a minute ago   Up About a minute (healthy)     ...
postgres            postgres            "docker-entrypoint.s…"   db                  About a minute ago   Up About a minute (healthy)     ...
traefik             traefik:2.9         "/entrypoint.sh trae…"   traefik             About a minute ago   Up About a minute (healthy)     ...
vault               vault:latest        "docker-entrypoint.s…"   vault               About a minute ago   Up About a minute (unhealthy)   ...
```

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
Steps:

Manual:
0. `juju unregister jimm-dev`                                       - Unregister any other local JIMM you have.
1. `juju login jimm.localhost -c jimm-dev`                          - Login to local JIMM. You don't have to do this, you will be logged in automatically.
2. `juju bootstrap microk8s qa-controller`                          - Bootstrap a Q/A controller.
3. `go build ./cmd/jimmctl`                                         - Build CLI tool.
4. `juju switch jimm-dev`                                           - Switch back to JIMM controller.
5. `./jimmctl controller-info qa-controller ./qa-controller.yaml`   - Get Q/A controller info.
5.1. Modify qa-controller.yaml public address to "juju-apiserver:17070"
5.2. Run `docker compose exec -it jimm bash` and update the /etc/hosts to have the api-addresses point to "juju-apiserver"
6. `./jimmctl add-controller ./qa-controller.yaml`                  - Add Q/A controller to JIMM.
7. `juju update-credentials microk8s --controller jimm-dev`         - Add client credentials for qa-controller's cloud (microk8s) to JIMM's controller credential list. 
8. `juju add-model test`                                            - Adds a model to qa-controller via JIMM.

Semi-automated:
0. `juju unregister jimm-dev`                                       - Unregister any other local JIMM you have.
1. `juju login jimm.localhost -c jimm-dev`                          - Login to local JIMM. (If you name the controller jimm-dev, the script will pick it up!)
2. `juju bootstrap microk8s qa-controller`                          - Bootstrap a Q/A controller. (If you name the controller qa-controller and the cloud is microk8s, the script will pick it up!)
3. `./local/jimm/add-controller.sh`                                 - A local script to do many of the manual steps for us. See script for more details.
4. `juju add-model test`                                            - Adds a model to qa-controller via JIMM.

# Helpful tidbits!
> Note: For any secure step to work, ensure you've run the local traefik certs script!

- To access vault UI, the URL is: `http://localhost:8200/ui` and the root key is `token`.
- The WS API for JIMM Controller is under: `ws://localhost:17070` (http direct) and `wss://jimm.localhost` for secure.
- You can verify local deployment with: `curl http://localhost:17070/debug/status` and `curl https://jimm.localhost/debug/status`
- Traefik is available on `http://localhost:8089`.