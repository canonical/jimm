---
myst:
  html_meta:
    description: "Learn how to create, rename, and manage groups in JAAS to organize users and control access to models, clouds, and controllers. Includes migration to IdP groups."
---

(manage-groups)=
# Manage groups
> Who: JIMM controller admin
>
> See also: {ref}`group`

```{important}
**Deprecation notice — migration to IdP groups**

JAAS groups (created with `juju add-group`) are deprecated and will be **removed** in the next major release of JAAS on the `v4` track. They are replaced by **groups managed by the Identity Provider**.

With Identity Provider (IdP) groups you no longer need to create groups in JAAS or assign users to them, group membership is managed by the IdP. In JAAS you only need to **assign permissions to an IdP group**, and every member of group inherits the permission automatically.

JAAS groups remain fully supported on the current `v3` track. If you are starting a new deployment, prefer IdP groups from the outset. If you already use JAAS groups, see {ref}`migrate-to-IdP-groups` below for the migration path.
```

````{dropdown} Preview an example workflow
```text
# Create a group:
juju add-group A

# Verify that the group has been created successfully:
juju list-groups

# Give the members of the group write access to test-model-1:
juju add-permission group-B#member writer model-test-ctl-1/test-model-1

# Rename the role to something more suitable:
juju rename-group model-writers

# Add users to the group:
juju add-permission user-alice@canonical.com member group-A
juju add-permission user-bob@canonical.com member group-B

# Verify that user Alice has indeed inherited the group's write access to test-model-1:
juju check-permission user-alice@canonical.com writer model-test-ctl-1/test-model-1

# Create another group B and make members of group A also members of group B:
juju add-permission group-A#member member group-B
...
```
````

The sections below describe the **JAAS groups** workflow, which is still supported on the `v3` track. For the IdP groups workflow and the migration from JAAS groups to IdP groups, see {ref}`IdP-groups` and {ref}`migrate-to-IdP-groups`.

(add-a-group)=
## Add a group

To add a new group to your JIMM controller, use the `add-group` command followed by the name you want to assign to the group. For example:

```text
juju add-group A
```

> See more: {doc}`juju add-group <../reference/jaas-plugin>`

(view-all-the-current-groups)=
## View all the current groups

To view all the current groups, run the `list-groups` command. For example:

```text
juju list-groups [options]
```

> See more: {doc}`juju list-groups <../reference/jaas-plugin>`

(add-a-user-to-a-group)=
## Add a user to a group

To add a user to a group, add a `member` permission between the user and the group. For example:

```text
juju add-permission user-alice@canonical.com member group-mygroup
juju add-permission group-groupA#member member group-groupB
juju add-permission user-everyone@external member group-mygroup
```

> See more: {ref}`manage-permissions`


(rename-a-group)=
## Rename a group

To rename a group, run the `rename-group` command followed by the old name and the new name. For example:

```text
juju rename-group TeamA TeamB
```

> See more: {doc}`juju rename-group <../reference/jaas-plugin>`

(remove-a-group)=
## Remove a group

To remove a group from a JIMM controller, run the `remove-group` command followed by the name of the group. For example:

```text
juju remove-group TeamB
```

> See more: {doc}`juju remove-group <../reference/jaas-plugin>`

(IdP-groups)=
## About IdP groups

> See also: {ref}`group`

An **IdP group** is a group that is managed by your Identity Provider (IdP), not by JAAS. When a user logs in to JAAS, the IdP includes an extra claim in the access token that lists the groups the user belongs to. JAAS reads these claims and treats each value as an IdP group the user is a member of.

Compared to JAAS groups, the key difference is that **JAAS no longer creates groups or assigns users to them**. In JAAS you only assign permissions (e.g. `administrator` on a model) to an IdP group, then every user who logs in with that group claim inherits the permission automatically.

IdP groups are referenced using the `idpgroup` tag and require the `#member` userset, just like JAAS groups. For example:

```text
# Grant administrator access on a model to every member of the "canonical" IdP group:
juju add-permission idpgroup-canonical#member administrator model-mycontroller/mymodel
```

> See more: {doc}`juju add-permission <../reference/jaas-plugin>`

### Limitations when checking indirect access

IdP group membership is provided in the group claim when a user logs in. As a result, commands that need to evaluate another user's permissions cannot determine that user's indirect access through an IdP. 

For example, `juju jaas check-permission` cannot reliably check a user's access inherited from an IdP group, and `juju show-model` does not list users whose access comes through IdP group membership. The same limitation applies to any command that relies on another user's permissions.

(configure-jimm-IdP-groups)=
## Configure JIMM's charm to accept group claims
There are two ways to configure JIMM's charm for IdP group claims, and it depends on the way the Identity Platform was configured.
If the identity platform was configured to emit the group claim by default you will need to change the `oauth-group-claim-key` charm config with the key where JIMM will find the group claim (e.g. `groups`).
If the identity platform is configured to _optionally_ offer the group claim then additionally configure both `oauth-client-credential-scopes` and `oauth-optional-scopes` (e.g. `groups`) to request group claims when authenticating client-credentials and users, respectively.

(migrate-to-IdP-groups)=
## Migrate from JAAS groups to IdP groups

JAAS groups are deprecated and will be removed in the `v4` release of JAAS. To migrate, move group membership management to your IdP and replace JAAS group permission assignments with IdP-group permission assignments.

The high-level migration steps are:

1. **Configure your IdP** to emit a group claim (e.g. `groups`) that contains the group names you want to use in JAAS. The exact configuration depends on your IdP (e.g. Canonical Identity Platform / Keycloak, Auth0, etc.).
2. **Map the existing JAAS groups** in JAAS to the group names the IdP will emit. For each local group, make sure the IdP emits a group with a matching name, and assign the same users to that group in the IdP.
3. **Re-assign permissions** from the local group to the IdP group. For every permission previously granted to `group-<name>#member`, grant the same permission to `idpgroup-<name>#member`.
4. **Remove the JAAS group** permission assignments and the local group itself once the IdP group is in use and users have confirmed they inherit the expected access.

### Terraform migration example

The following example shows how to migrate a typical local-group setup to IdP groups using the [Terraform Provider for Juju](https://documentation.ubuntu.com/terraform-provider-juju/).

**Before — JAAS groups (deprecated, `v3` only):**

```hcl
# Create a group in JAAS:
resource "juju_jaas_group" "test_group" {
  name = "test-group"
}

# Assign users to the JAAS group:
resource "juju_jaas_access_group" "development" {
  group_id = juju_jaas_group.test_group.uuid
  access   = "member"
  users    = ["jimm-group-user@canonical.com"]
}

# Grant the JAAS group administrator access on a model:
resource "juju_jaas_access_model" "development" {
  model_uuid = juju_model.test_model.uuid
  access     = "administrator"
  groups     = [juju_jaas_group.test_group.uuid]
}
```

**After — IdP groups (`v3` and `v4`):**

```hcl
# No juju_jaas_group or juju_jaas_access_group resources are needed.
# Group membership is managed by the IdP and sent as a claim at login.

# Grant the IdP group "canonical" administrator access on a model:
resource "juju_jaas_access_model" "development" {
  model_uuid = juju_model.test_model.uuid
  access     = "administrator"
  idp_groups = ["canonical"]
}
```

With the IdP groups setup:

- There is no `juju_jaas_group` resource, the group is not created in JAAS.
- There is no `juju_jaas_access_group` resource, user-to-group membership is managed by the IdP.
- The `juju_jaas_access_model` resource uses `idp_groups` instead of `groups` to reference the IdP group by name.

```{note}
When you apply the migration, remove the `juju_jaas_group` and `juju_jaas_access_group` resources from your Terraform configuration and run `terraform apply`. Existing JAAS groups and their permission assignments can also be removed with the `juju remove-group` command once the IdP group permissions are in place.
```
