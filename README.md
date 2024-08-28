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
- Federated user management to an external identity provider using OAuth 2.0 and OIDC.
- Fine grained access control with the ability to create user groups.
- Simplified networking, exposing a single gateway into your Juju estate.
- The ability to query for information across all your Juju controllers.

For a full overview of the capabilties, check out 
[the docs](https://canonical-jaas-documentation.readthedocs-hosted.com/en/latest/explanation/jaas_overview/).


## Dependencies

The project uses [Go modules](https://golang.org/cmd/go/#hdr-Module_maintenance) to manage 
Go dependencies. **Note: Go 1.11 or greater needed.**

A brief explanation of the various services that JIMM depends on is below:
- Vault: User's cloud-credentials are stored in Vault. Cloud-credentials are API keys that 
enable Juju to communicate with a cloud's API.
- Postgres: A majority of JIMM's state is stored in Postgres.
- OpenFGA: A cloud-native authorisation tool where authorisation rules are stored and queried 
using relation based access control.
- IdP: An identity provider, utilising OAuth 2.0 and OIDC, JIMM delegates authentication to a 
separate identity service.

## JIMM versioning

JIMM v0 and v1 follow a different versioning strategy than future releases. JIMM v0 was the initial
release and used MongoDB to store state. JIMM v1 was an upgrade that switched to using PostgreSQL 
for storing state but still retained similar functionality to v0.  
These versions worked with Juju v2 and v3.

Subsequently JIMM introduced a large shift in how the service worked:
- JIMM now acts as a proxy between all client and Juju controller interactions. Previously 
users were redirected to a Juju controller.
- Juju controllers trust a public key served by JIMM.
- JIMM acts as an authorisation gateway creating trusted short-lived JWT tokens to authorize 
user actions against Juju controllers.

The above work encompassed a breaking change and required changes in Juju (requiring a 
Juju controller of at least version 3.3). 

Further, to better align the two projects, JIMM's versioning now aligns with Juju.

As a result of this, there is no JIMM v2 and instead from JIMM v3, the versioning strategy 
we follow is to match JIMM's major version to the corresponding Juju major version we support.

As an example, JIMM v3 is intended to support Juju v3 controllers AND the last minor version 
of the previous major (Juju v2.9) for migration purposes.


## Binaries

This repo contains 3 binaries:
- jimmsrv: The JIMM server.
- jimmctl: A CLI tool for administrators of JIMM to view audit logs, manage permissions, etc. 
Available as a snap.
- jaas: A plugin for the Juju CLI, extend the base set of command with extra functionality when
communicating with a JAAS environment. 

## Development environment

See [here](./local/README.md) on how to get started.

## Testing

See [here](./CONTRIBUTING.md) on how to get started.
