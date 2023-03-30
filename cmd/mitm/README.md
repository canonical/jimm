# Man-in-the-middle Proof of Concept Service for a Juju Controller

## Introduction

This is a proof of concept service and is **NOT production ready**. Once started, service will proxy all connections to the Juju controller specified in the configuration file by opening a websocket connection to the controller and sending the same payload it receive from the client, then waiting for the Juju controller to reply before sending the reply back to the client.

## Configuration

To run the service you will need a running Juju controller a self-signed key, certificate and CA certificate and a configuration file that follows the following template:
```
    ca-cert-file: <CA cert file path>
    cert-file: <cert file path>
    key-file: <key file path>
    hostname: 127.0.0.1:443
    controller:
        uuid: <uuid of the controller>
        api-endpoints: [<list of controller endpoints>]
        ca-cert: <controller's ca cert>
```
To generate the self signed CA, cert and key please follow one of the many tutorials Google has on offer. Once can also use the tool inside tools/make-certs.go. This can be done with `go build ./tools` which will generate a `caCert.crt`, a `cert.crt` and a `myKey.key`. Point your config file to these files. Next you can run the following commands to add the CA cert to your cert pool.
```
sudo cp caCert.crt /usr/local/share/ca-certificates
sudo update-ca-certificates 
```

The controller part, you can get from your local controllers.yaml file located in `~/.local/share/juju`. Find the entry corresponding to the controller you intend to use and copy the needed data into the configuration file. Note that setting up the controller is described in further detail below.

## Controller Setup
To setup the Juju controller from a development branch, clone the desired repo/fork and switch to the desired branch and run 
`make go-install`
Then, assuming `$GOPATH/bin` is in your PATH (you can confirm this by running `juju --version` to confirm the juju version is the same as the built source).
Now you can bootstrap a controller by running,
```
juju bootstrap --config login-token-refresh-url=https://127.0.0.1 localhost <controller-name>
```

Note: The path to the mitm service cannot contain a port otherwise the Juju controller will report the following:
`machine-0: 10:51:23 WARNING juju.apiserver failed to refresh jwt cache: failed to fetch "127.0.0.1:443/.well-known/jwks.json": failed to fetch "127.0.0.1:443/.well-known/jwks.json": parse "127.0.0.1:443/.well-known/jwks.json": first path segment in URL cannot contain colon`

After the controller has started we will add a [proxy](https://linuxcontainers.org/lxd/docs/master/reference/devices_proxy/) to the lxc container to allow the controller to make requests to the host's mitm service in order to obtain the JWKS key set.
Run 
```
lxc list # Get name of the machine running the juju controller, this will resemble "juju-d5ede3-0"
export TEST_JUJU_CTRL=<instance-name> # This will make subsequent requests easier
lxc config device add "${TEST_JUJU_CTRL}" myproxy proxy listen=tcp:0.0.0.0:443 connect=tcp:127.0.0.1:443 bind=instance
```

Next the CA Cert we generated previously needs to be added to the controller. This can be done with the following commands
```
# Copy the cert into the container and update
lxc file push ./caCert.crt "${TEST_JUJU_CTRL}"/
lxc shell "${TEST_JUJU_CTRL}"
cp /caCert.crt /usr/local/share/ca-certificates
update-ca-certificates
exit
# Restart the container for the controller to update its certs.
lxc stop "${TEST_JUJU_CTRL}"
lxc start "${TEST_JUJU_CTRL}"
```

## JWT

When forwarding the `Login` call on the `Admin` facade, the service will add a `token` field to the `LoginRequest`, which holds a base64 encoded JWT with the following claims:
```
    <controller tag>: admin
    <model tag>: admin # if a model UUID is available in the request path
```

The service also serves a set of `.well-known` endpoints serving the JWKS that can be used to validate the JWT.

### In-memory JWT storage

Since the service currently stores the JWKS data in memory, every time we restart the service a new JWKS set will be created. Since the juju controller caches the JWKS data, every time we restart the service, we also need to restart the Juju controller. Run:
```
    # Restart the container for the controller to update its certs.
    lxc stop <instance-name>
    lxc start <instance-name>
```

## Running the service

To run the service run: 
```
go build ./cmd/mitm
sudo ./mitm <path to configuration yaml>
```

## Testing the service

To test a juju client with the service run `juju login localhost:443 -c mitm --debug`
This will connect to the mitm server at localhost:443 and you should see trace logs come through on the terminal running the man-in-the-middle-server.