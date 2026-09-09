# Dialing backing controllers

JIMM dials backing controllers along two axes:

- **Connection scope**: model-scoped vs controller-scoped.
- **Identity/permissions**: the user's real permissions (`AsUser`),
  the user's identity with forced superuser (`AsSuperuser`), or JIMM's
  own service identity with no user (`AsService`).

Prefer `AsUser` wherever possible. `AsSuperuser` preserves the user's
identity but forces superuser permissions — use it when the backing
facade needs more than the caller holds. `AsService` uses JIMM's admin
identity — use it only when JIMM is the acting party and no user
identity is needed. See the `Dialer` interface godoc for method
semantics.

## Remaining dial-as-service uses

### Migration machinery

User-triggered, but JIMM is the acting party.

- `migrationtarget.go`: Abort, CheckMachines, Prechecks, Import,
  AdoptResources, LatestLogTime, Activate

## Remaining dial-as-superuser uses

### JIMM-admin operations

User-triggered, but the admin holds no OpenFGA relations on the backing
controller's resources, so there are no claims to mint.

- `model.go` `UpgradeController`: upgrades the controller model, which
  is never related to users in OpenFGA
- `controller.go` `ControllerConfig`
- `controller.go` `AddController`: controller not yet persisted
- `controller.go` `fetchModelInfo`: model import, JIMM is acting party

### Facade permission gaps

JIMM enforces authorization, but the backing facade needs more than
the caller holds.

- `cloud.go`: add/manage/remove hosted clouds
- `model.go` `ChangeModelCredential`: forced credential update
  (`force=true` requires controller superuser)
- `modelbuilder.go`: forced credential update on model create.
  `GrantJIMMModelAdmin` (JUJU-8869)
- `applicationoffer.go` `queryControllersForOffers`: spans many models,
  a single caller-scoped JWT can't satisfy Juju's per-model check.
  Results re-authorized via OpenFGA
- `jujuapi/streamcontrollerproxy.go`: migration log-transfer stream
