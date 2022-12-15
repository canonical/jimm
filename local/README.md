# Local Development

## Starting the environment
1. Run `make pull/candid` to get a local image of candid (this is subject to change!)
2. docker compose up

The services included are:
- JIMM
- Vault
- Postgres
- OpenFGA

> Any changes made inside the repo will automatically restart the JIMM server via a volume mount. So there's no need
to re-run the compose continuously, but note, if you do bring the compose down, remove the volumes otherwise
vault will not behave correctly, this can be done via `docker compose down -v`

If all was successful, you should seen an output similar to:
```
NAME                COMMAND                  SERVICE             STATUS                PORTS
candid              "/candid.sh"             candid              running (healthy)     0.0.0.0:8081->8081/tcp, :::8081->8081/tcp
jimmy               "/go/bin/air"            jimm                running (healthy)     0.0.0.0:17070->8080/tcp, :::17070->8080/tcp
migrateopenfga      "/openfga migrate --…"   migrateopenfga      exited (0)            
openfga             "/openfga run --data…"   openfga             running (unhealthy)   0.0.0.0:3000->3000/tcp, :::3000->3000/tcp, 0.0.0.0:8080->8080/tcp, :::8080->8080/tcp
postgres            "docker-entrypoint.s…"   db                  running (healthy)     0.0.0.0:5432->5432/tcp, :::5432->5432/tcp
vault               "docker-entrypoint.s…"   vault               running (unhealthy)   0.0.0.0:8200->8200/tcp, :::8200->8200/tcp
```

Now please checkout the [Authentication Steps](#authentication-steps) to authenticate postman for local testing & Q/A.
## Authentication Steps
1. Run `make get-local-auth`
2. Head to postman and follow the instructions given by get-local-auth.

## Facades in Postman

You will see JIMM's controller WS API broken up into separate WS requests.
This is intentional.
Inside of each WS request will be a set of `saved messages` (on the right-hand side), these are the calls to facades for the given API under that request.

The `request name` represents the literal WS endpoint, i.e., `API = /api`.

> Remember to run the `Login` message when spinning up a new WS connection, otherwise you will not be able to send subsequent calls to this WS.

## Adding Controllers
TODO.

### Helpful tidbits!
- To access vault UI, the URL is: `http://localhost:8200/ui` and the root key is `token`.
- The WS API for JIMM Controller is under: `ws://localhost:17070`.
- You can verify local deployment with: `curl http://localhost:17070/debug/status`
