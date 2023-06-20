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

Manual:
1. `juju unregister jimm-dev`                                       - Unregister any other local JIMM you have.
2. `juju login jimm.localhost -c jimm-dev`                          - Login to local JIMM. You don't have to do this, you will be logged in automatically.
3. `juju bootstrap localhost qa-controller --config allow-model-access=true --config login-token-refresh-url=https://jimm.localhost`                          - Bootstrap a Q/A controller.
   1. run `lxc list` to name of the machine running the juju controller, this will resemble "juju-d5ede3-0"
   2. export name of the machine using `export TEST_JUJU_CTRL=<instance-name>`
   3. create a proxy on the controller machine that will enable controller to reach jimm by running `lxc config device add "${TEST_JUJU_CTRL}" myproxy proxy listen=tcp:0.0.0.0:443 connect=tcp:127.0.0.1:443 bind=instance`
   4. we also need to push the CA cert that was used to create JIMM's certificate to the machine by running `lxc file push local/traefik/certs/ca.crt "${TEST_JUJU_CTRL}"/usr/local/share/ca-certificates/`
   5. now go into the controller `lxc shell "${TEST_JUJU_CTRL}"`
   6. and run `update-ca-certificates` to update machine's CA certs
   7. we also want to add an entry to `/etc/hosts` that will allow the controller to correctly resolve `jimm.localhost` and use the proxy we created in step 3. Run `echo "127.0.0.1 jimm.localhost" >> /etc/hosts`
   8. next we need to start and stop the controller by running `lxc stop "${TEST_JUJU_CTRL}"` and then `lxc start "${TEST_JUJU_CTRL}"` because controller fetches the jwks from jimm on startup at which point the proxy had not yet been set up.
4. `go build ./cmd/jimmctl`     
5. `./jimmctl controller-info qa-controller ./qa-controller.yaml`   - Get Q/A controller info.
   1. Modify qa-controller.yaml public address to "juju-apiserver:17070"
   2. Run `docker compose exec -it jimm bash` and update the /etc/hosts to have the api-addresses point to "juju-apiserver"                                    - Build CLI tool.
6. `juju switch jimm-dev`                                           - Switch back to JIMM controller.
7. `./jimmctl add-controller ./qa-controller.yaml`                  - Add Q/A controller to JIMM.
8. `juju update-credentials localhost --controller jimm-dev`         - Add client credentials for qa-controller's cloud (localhost) to JIMM's controller credential list. 
9. `juju add-model test`                                            - Adds a model to qa-controller via JIMM.

Semi-automated:
0. `juju unregister jimm-dev`                                       - Unregister any other local JIMM you have.
1. `juju login jimm.localhost -c jimm-dev`                          - Login to local JIMM. (If you name the controller jimm-dev, the script will pick it up!)
2. `juju bootstrap localhost qa-controller --config allow-model-access=true --config login-token-refresh-url=https://jimm.localhost`                          - - Bootstrap a Q/A controller. (If you name the controller qa-controller and the cloud is lxd or microk8s, the script will pick it up!)
   1. run `lxc list` to name of the machine running the juju controller, this will resemble "juju-d5ede3-0"
   2. export name of the machine using `export TEST_JUJU_CTRL=<instance-name>`
   3. create a proxy on the controller machine that will enable controller to reach jimm by running `lxc config device add "${TEST_JUJU_CTRL}" myproxy proxy listen=tcp:0.0.0.0:443 connect=tcp:127.0.0.1:443 bind=instance`
   4. we also need to push the CA cert that was used to create JIMM's certificate to the machine by running `lxc file push local/traefik/certs/ca.crt "${TEST_JUJU_CTRL}"/usr/local/share/ca-certificates/`
   5. now go into the controller `lxc shell "${TEST_JUJU_CTRL}"`
   6. and run `update-ca-certificates` to update machine's CA certs
   7. we also want to add an entry to `/etc/hosts` that will allow the controller to correctly resolve `jimm.localhost` and use the proxy we created in step 3. Run `echo "127.0.0.1 jimm.localhost" >> /etc/hosts`
   8. next we need to start and stop the controller by running `lxc stop "${TEST_JUJU_CTRL}"` and then `lxc start "${TEST_JUJU_CTRL}"` because controller fetches the jwks from jimm on startup at which point the proxy had not yet been set up.
3. `./local/jimm/add-controller.sh`                                 - A local script to do many of the manual steps for us. See script for more details.
4. `juju add-model test`                                            - Adds a model to qa-controller via JIMM.

# Helpful tidbits!
> Note: For any secure step to work, ensure you've run the local traefik certs script!

- To access vault UI, the URL is: `http://localhost:8200/ui` and the root key is `token`.
- The WS API for JIMM Controller is under: `ws://localhost:17070` (http direct) and `wss://jimm.localhost` for secure.
- You can verify local deployment with: `curl http://localhost:17070/debug/status` and `curl https://jimm.localhost/debug/status`
- Traefik is available on `http://localhost:8089`.
