# Juju Intelligent Model Manager

This service provides the ability to manage multiple juju models. It is
considered a work in progress.

## Installation

To start using JIMM, first ensure you have a valid Go environment,
then run the following:

    go get github.com/CanonicalLtd/jem
    cd $GOPATH/src/github.com/CanonicalLtd/jem

## Go dependencies

The project uses godeps (https://launchpad.net/godeps) to manage Go
dependencies. To install this, run:

    go get launchpad.net/godeps

After installing it, you can update the dependencies
to the revision specified in the `dependencies.tsv` file with the following:

    make deps

Use `make create-deps` to update the dependencies file.

## Development environment

A couple of system packages are required in order to set up a development
environment. To install them, run the following:

    make sysdeps

At this point, from the root of this branch, run the command::

    make install

The command above builds and installs the JIMM binaries, and places
them in `$GOPATH/bin`. This is the list of the installed commands:

- jemd: start the JIMM server;
- jaas-admin: perform admin commands on JIMM;

## Testing

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
