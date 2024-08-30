# JIMM - Juju Intelligent Model Manager

[comment]: <> (Update the chat link below with a JIMM specific room)
<h4 align="center">
    <a href="https://app.element.io/#/room/#charmhub-juju:ubuntu.com">Chat</a> |
    <a href="https://canonical-jaas-documentation.readthedocs-hosted.com/en/latest/">Docs</a> |
    <a href="https://github.com/canonical/jimm-k8s-operator/">Charm</a>
</h4>

JIMM is a Go based webserver used to provide extra functionality on top of Juju controllers. 
If you are unfamiliar with Juju, we suggest visiting the [Juju docs](https://juju.is/) - 
the open source orchestration engine for software operators.

JIMM provides the ability to manage multiple Juju controllers from a single location with 
enhanced enterprise functionality.

JIMM is the central component of JAAS (Juju As A Service), where JAAS is a set of services 
acting together to enable storing state, storing secrets and auth.

## Features

JIMM/JAAS provides enterprise level functionality layered on top of your Juju controller like:
- Federated login via an external identity provider using OAuth2.0 and OIDC.
- Fine grained access control with the ability to create user groups.
- A single gateway into your Juju estate.
- The ability to query for information across all your Juju controllers.

For a full overview of the capabilties, check out 
[the docs](https://canonical-jaas-documentation.readthedocs-hosted.com/en/latest/explanation/jaas_overview/).


## Dependencies

The project uses [Go modules](https://golang.org/cmd/go/#hdr-Module_maintenance) to manage 
Go dependencies. **Note: Go 1.11 or greater needed.**

A brief explanation of the various services that JIMM depends on is below:
- [Vault](https://www.vaultproject.io/): User cloud-credentials and private keys are stored in Vault. Cloud-credentials are API keys that 
enable Juju to communicate with a cloud's API.
- [PostgreSQL](https://www.postgresql.org/): All non-sensitive state is stored in Postgres.
- [OpenFGA](https://openfga.dev/): A distributed authorisation server where authorisation rules are stored and queried 
using relation based access control.
- IdP: An identity provider which supports OAuth2.0 and OIDC.

## JIMM versioning

The versioning strategy we follow is to match JIMM's major version to the corresponding
Juju major version we support.

Additionally JIMM will also support Juju's last minor version of the previous major to 
support model migrations.

E.g. JIMM v3 supports Juju v3 controllers AND the last minor version 
of the previous major, v2.9.

For more information on JIMM's history and previous version strategy see [here](./doc/versioning.md).

## Binaries

This repository contains 3 binaries:
- jimmsrv: The JIMM server.
- jimmctl: A CLI tool for administrators of JIMM to view audit logs, manage permissions, etc. 
Available as a snap.
- jaas: A plugin for the Juju CLI, extend the base set of command with extra functionality when
communicating with a JAAS environment. 

## Development environment

See [here](./local/README.md) on how to get started.

## Testing

See [here](./CONTRIBUTING.md) on how to get started.
