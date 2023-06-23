# Local Development & Testing

This doc is intended to help those new to JIMM get up and running
with the local Q/A environment. This environment is additionally
used for integration testing within the JIMM test suite.

# Starting the environment
1. Ensure you have docker above v18, confirm this with `docker --version`
2. Ensure you are in the root JIMM directory.
3. Run `make pull/candid` to get a local image of candid (this is subject to change!)
4. Run `cd local/traefik/certs; ./certs.sh; cd -`, this will setup some self signed certs and add them to your cert pool.
5. Run `touch ./local/vault/approle.json && touch ./local/vault/roleid.txt`
6. Run `make version/commit.txt` to populate the repo with the git commit info.
7. Run `make version/version.txt` to populate the repo with the git version info.
8. `docker compose --profile dev up` if you encounter an error like "Error response from daemon: network ... not found" then the command `docker compose --profile dev up --force-recreate` should help.

After this initial setup, subsequent use of the compose can be done with `docker compose --profile dev up --force-recreate`

The services included are:
- JIMM (only started in the dev profile)
- Vault
- Postgres
- Traefik

> Any changes made inside the repo will automatically restart the JIMM server via a volume mount. So there's no need
to re-run the compose continuously, but note, if you do bring the compose down, remove the volumes otherwise
vault will not behave correctly, this can be done via `docker compose down -v`

If all was successful, you should seen an output similar to:
```
NAME                COMMAND                  SERVICE             STATUS                PORTS
candid              candid:latest       "/candid.sh"             candid              About a minute ago   Up About a minute (healthy)     ...
jimmy               cosmtrek/air        "/go/bin/air"            jimm                About a minute ago   Up 54 seconds (healthy)         ...
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

## Prerequisited

1. Make sure you are using latest juju from their develop branch (at the moment, we're using 3.2-beta1).
2. change jujuClientVersion in `internal/jujuclient/dial.go` to **3.2-beta1** by running: ``sed -i 's/jujuClientVersion = "2.9.42"/jujuClientVersion = "3.2-beta1"/g' ./internal/jujuclient/dial.go``

Steps:

1. `juju unregister jimm-dev`                                       - Unregister any other local JIMM you have.
2. `juju login jimm.localhost -c jimm-dev`                          - Login to local JIMM. (If you name the controller jimm-dev, the script will pick it up!)
3. `./local/jimm/setup-controller.sh`                               - Performs controller setup.
4. `./local/jimm/add-controller.sh`                                 - A local script to do many of the manual steps for us. See script for more details.
5. `juju add-model test`                                            - Adds a model to qa-controller via JIMM.

# Helpful tidbits!
> Note: For any secure step to work, ensure you've run the local traefik certs script!

- To access vault UI, the URL is: `http://localhost:8200/ui` and the root key is `token`.
- The WS API for JIMM Controller is under: `ws://localhost:17070` (http direct) and `wss://jimm.localhost` for secure.
- You can verify local deployment with: `curl http://localhost:17070/debug/status` and `curl https://jimm.localhost/debug/status`
- Traefik is available on `http://localhost:8089`.
