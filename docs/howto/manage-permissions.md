---
myst:
  html_meta:
    description: "Learn how to add, revoke, and check permissions in JAAS for users, groups, roles, and resources including controllers, clouds, and models."
---

(manage-permissions)=
# Manage permissions
> See first: {external+juju:ref}`Juju | Juju access levels <user-access-levels>`
>
> See also: {ref}`jaas-authorization`

(add-a-permission)=
## Add a permission

To add a permission between an entity A (always a user, whether identified directly or through a group/role) and an entity B (group, role, or resource -- controller, cloud, model, or application offer), run the `add-permission` command followed by A (in tag notation or alternatives), the desired B-supported permission, and B (in tag notation). For example:

```text
# Make Alice cloud admin:
juju add-permission user-alice@canonical.com administrator cloud-mycloud

# Let all users add models on the cloud:
juju add-permission user-everyone@external can_addmodel cloud-mycloud

# Make Alice model admin:
juju add-permission user-alice@canonical.com administrator model-mycontroller/mymodel

# Let all users with role myrole have read access to model mymodel:
juju add-permission role-myrole#assignee reader model-mycontroller/mymodel

# Let Alice consume offer myoffer:
juju add-permission user-alice@canonical.com consumer applicationoffer-mycontroller/mymodel.myoffer

# Add Bob and Cindy to the mygroup group:
juju add-permission user-bob@canonical.com member group-mygroup
juju add-permission user-cindy@canonical.com member group-mygroup

# Let everyone in group mygroup add models that will use resources from cloud my-cloud:
juju add-permission group-mygroup#member can-addmodel cloud-mycloud
```

|entity A | permission| entity B|
|-|-|-|
|{ref}`user tag or alternatives except for role assignee <user-tag>` |{ref}`role-permission-assignee`|{ref}`role tag <role-tag>`|
|{ref}`user tag or alternatives <user-tag>` |{ref}`group-permission-member`| {ref}`group tag <group-tag>`|
|{ref}`user tag or alternatives <user-tag>` |{ref}`controller-permission-can-addmodel`| {ref}`controller tag <controller-tag>`|
|{ref}`user tag or alternatives <user-tag>` |{ref}`controller-permission-audit-log-viewer`| {ref}`controller tag <controller-tag>`|
|{ref}`user tag or alternatives <user-tag>` |{ref}`controller-permission-administrator`| {ref}`controller tag <controller-tag>`|{ref}<-permission-
|{ref}`user tag or alternatives <user-tag>` |{ref}`cloud-permission-can-addmodel`| {ref}`cloud tag <cloud-tag>`|
|{ref}`user tag or alternatives <user-tag>` |{ref}`cloud-permission-administrator`| {ref}`cloud tag <cloud-tag>`|
|{ref}`user tag or alternatives <user-tag>` |{ref}`model-permission-reader`|{ref}`model tag <model-tag>`|
|{ref}`user tag or alternatives <user-tag>` |{ref}`model-permission-writer`|{ref}`model tag <model-tag>`|
|{ref}`user tag or alternatives <user-tag>` |{ref}`model-permission-administrator`|{ref}`model tag <model-tag>`|
|{ref}`user tag or alternatives <user-tag>` |{ref}`offer-permission-reader` | {ref}`offer tag <offer-tag>` |
|{ref}`user tag or alternatives <user-tag>` |{ref}`offer-permission-consumer` | {ref}`offer tag <offer-tag>`|
|{ref}`user tag or alternatives <user-tag>` |{ref}`offer-permission-administrator` | {ref}`offer tag <offer-tag>` |

For any given resource, permissions are currently hierarchical and some permissions are implicit -- e.g., given a cloud associated with a controller and a model associated with the cloud, a controller `administrator` entails cloud `administrator` entails cloud `can_addmodel`.

> See more: {doc}`juju add-permission <../reference/jaas-plugin>`


(verify-a-permission)=
## Verify a permission

Given two entities A and B, to verify that there is a specific permission between them, run the `check-permission` command followed by the tag of A, the permission, and the tag of B. For example:

```text
juju check-permission user-alice@canonical.com administrator controller-aws-controller-1
```

> See more: {doc}`juju check-permission <../reference/jaas-plugin>`

(view-all-the-current-permissions)=
## View all the current permissions

To view all the current permissions, run the `list-permissions` command. For example:

```text
juju list-permissions [options]
```

> See more: {doc}`juju list-permissions <../reference/jaas-plugin>`


(remove-a-permission)=
## Remove a permission

Given two entities A and B and a pre-existing permission between them, to remove the permission, run the `remove-permission` command followed by the tag of A, the permission, and the tag of B. For example:

```text
juju remove-permission user-alice@canonical.com member group-mygroup
```

> See more: {ref}`view-all-the-current-permissions`, {doc}`juju remove-permission <../reference/jaas-plugin>`

