# Local Development

## Starting the environment
1. Run `make pull/candid` to get a local image of candid (this is subject to change!)
2. docker compose up

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
