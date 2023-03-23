# Man-in-the-middle Proof of Concept Service for a Juju Controller

## Introduction

This is a proof of concept service and is **NOT production ready**. Once started, service will proxy all connections to the Juju controller specified in the configuration file by opening a websocket connection to the controller and sending the same payload it receive from the client, then waiting for the Juju controller to reply before sending the reply back to the client.

## Configuration

To run the service you will need a running Juju controller a self-signed key, certificate and CA certificate and a configuration file that follows the following template:
```
    ca-cert-file: <CA cert file path>
    cert-file: <cert file path>
    key-file: <key file path>
    controller:
        uuid: <uuid of the controller>
        api-endpoints: [<list of controller endpoints>]
        ca-cert: <controller's ca cert>
```
To generate the self signed CA, cert and key please follow one of the many tutorials Google has on offer. The controller part, you can get from your local controllers.yaml file located in `~/.local/share/juju`. Find the entry corresponding to the controller you intend to use and copy the needed data into the configuration file.

## JWT

When forwarding the `Login` call on the `Admin` facade, the service will add a `token` field to the `LoginRequest`, which holds a base64 encoded JWT with the following claims:
```
    <controller tag>: admin
    <model tag>: admin # if a model UUID is available in the request path
```

The service also serves a set of `.well-known` endpoints serving the JWKS that can be used to validate the JWT.


## Running the service

To run the service run: `go run main.go <path to configuration yaml>`.