# Juju Intelligent Model Manager

This service provides the ability to manage multiple juju models. It is
considered a work in progress.

## Installation

To start using JIMM, first ensure you have a valid Go environment,
then run the following:

    go get github.com/canonical/jimm

## Go dependencies

The project uses Go modules (https://golang.org/cmd/go/#hdr-Module_maintenance) to manage Go
dependencies. **Note: Go 1.11 or greater needed.**

## Development environment

### Local:
A couple of system packages are required in order to set up a development
environment. To install them, run the following:
`make sysdeps`

At this point, from the root of this branch, run the command:
`make install`

The command above builds and installs the JIMM binaries, and places
them in `$GOPATH/bin`. This is the list of the installed commands:

- jemd: start the JIMM server;
- jaas-admin: perform admin commands on JIMM;

### Docker compose:
See [here](./local/README.md) on how to get started.

## Testing

### Pre-requisite
As the juju controller internal suites start their our mongod instances, it is required to have juju-db (mongod).
This can be installed via: `sudo snap install juju-db`.
The latest JIMM has an upgraded dependency on Juju which requires in turn requires juju-db from channel `4.4/stable`,
 this can be installed with `sudo snap install juju-db --channel=4.4/stable`

The rest of the suite relies on PostgreSQL, OpenFGA and Hashicorp Vault which are dockerised
and as such you may simple run `docker compose up` to be integration test ready.
The above command won't start a dockerised instance of JIMM as tests are normally run locally. Instead, to start a 
dockerised JIMM that will auto-reload on code changes, simply run `docker compose --profile dev up`.

### Manual commands
The tests utilise [go.check](http://labix.org/gocheck) for suites and you may run tests individually like so: 
```bash
$ go test -check.f dialSuite.TestDialWithCredentialsStoredInVault`
$ go test -check.f MyTestSuite
$ go test -check.f "Test.*Works"
$ go test -check.f "MyTestSuite.Test.*Works"
```

For more verbose output, use `-check.v` and `-check.vv`


### Make
Run `make check` to test the application.
Run `make help` to display help about all the available make targets.

## Local QA

To start a local server for QA purposes do the following:

    sudo cp tools/jimmqa.crt /usr/local/share/ca-certificates
    sudo update-ca-certificates
    make server

This will start JIMM server running on localhost:8082 which is configured
to use https://api.staging.jujucharms.com/identity as its identity
provider.

To add the new JIMM to your juju environment use the command:

   juju login localhost:8082 -c local-jaas

To bootstrap a new controller and add it to the local JIMM use the
following commands:

    juju bootstrap --config identity-url=https://api.staging.jujucharms.com/identity --config allow-model-access=true <cloud>/<region> <controller-name>
    jaas-admin --jimm-url https://localhost:8082 add-controller <owner>/<controller-name>

