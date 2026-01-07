(controller)=
# Controller

> See first: {external+juju:ref}`Juju | Controller <controller>`
>
> See also: {ref}`manage-juju-controllers`

(controller-tag)=
## Controller tag

A controller tag has the following format:

```text
controller-<controller name>
```

(controller-permission)=
## Controller permission

A controller permission describes what an entity can do on a controller.

(list-of-controller-permissions)=
### List of controller permissions

(controller-permission-administrator)=
#### `administrator`

Abilities: Can do anything that it is possible to do at the level of a controller. This grants permissions to all resources that inherit from controller access.

(controller-permission-audit-log-viewer)=
#### `audit_log_viewer`

Abilities: Can read audit logs.