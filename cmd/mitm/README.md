# Man-in-the-middle Proof of Concept Service for a Juju Controller

## Introduction

This is a proof of concept service and is **NOT production ready**. Once started, service will proxy all connections to the Juju controller specified in the configuration file by opening a websocket connection to the controller and sending the same payload it receive from the client, then waiting for the Juju controller to reply before sending the reply back to the client.

## Configuration

To run the service you will need a running Juju controller a self-signed key, certificate and CA certificate and a configuration file that follows the following template:
```
    ca-cert-file: <CA cert file path>
    cert-file: <cert file path>
    key-file: <key file path>
    hostname: 127.0.0.1:17071
    controller:
        uuid: <uuid of the controller>
        api-endpoints: [<list of controller endpoints>]
        ca-cert: <controller's ca cert>
```
To generate the self signed CA, cert and key please follow one of the many tutorials Google has on offer. Once can also use the tool inside tools/make-certs.go. This can be done with `go build ./tools` which will generate a `caCert.crt`, a `cert.crt` and a `myKey.key`. Point your config file to these files. Next you can run the following commands to add the CA cert to your cert pool.
```
sudo cp tools/jimmqa.crt /usr/local/share/ca-certificates
sudo update-ca-certificates 
```

The controller part, you can get from your local controllers.yaml file located in `~/.local/share/juju`. Find the entry corresponding to the controller you intend to use and copy the needed data into the configuration file.

## Controller Setup
To setup the Juju controller from a development branch, clone the desired repo/fork and switch to the desired branch and run 
`make go-install`
Then, assuming `$GOPATH/bin` is in your PATH (you can confirm this by running `juju --version` to confirm the juju version is the same as the built source) you can bootstrap a controller with `juju bootstrap --config jwt-refresh-url=127.0.0.1:17071 localhost <controller-name>`

Note: There is currently an issue that the controller cannot hit the ip:port combo because of the following warning
`machine-0: 10:51:23 WARNING juju.apiserver failed to refresh jwt cache: failed to fetch "127.0.0.1:17071/.well-known/jwks.json": failed to fetch "127.0.0.1:17071/.well-known/jwks.json": parse "127.0.0.1:17071/.well-known/jwks.json": first path segment in URL cannot contain colon`

Another issue is that a controller in LXD will be unable to communicate to a server on localhost without some ip table tweaks.

## JWT

When forwarding the `Login` call on the `Admin` facade, the service will add a `token` field to the `LoginRequest`, which holds a base64 encoded JWT with the following claims:
```
    <controller tag>: admin
    <model tag>: admin # if a model UUID is available in the request path
```

The service also serves a set of `.well-known` endpoints serving the JWKS that can be used to validate the JWT.


## Running the service

To run the service run: `go run main.go <path to configuration yaml>`.

## Testing the service

To test a juju client with the service run `juju login localhost:17071 -c mitm --debug`
This will connect to the mitm server at localhost:17071 and you should see trace logs come through on the terminal running the man-in-the-middle-server.