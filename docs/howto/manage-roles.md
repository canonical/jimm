(manage-roles)=
# Manage roles
> Who: JIMM controller admin
>
> See also: {ref}`role`

```{note}
This guide assumes you have the `jaas` plugin installed.
See {doc}`here <../explanation/jaas-plugin>` for more information.
```

````{dropdown} Preview an example workflow

```text
# Create a role
juju add-role myrole

# Verify that the role has been created successfully:
juju list-roles

# Give the role admin access to a model:
juju add-permission role-model-admin#assignee administrator model-bob@canonical.com/foo

# Rename the role to better match its function:
juju rename-role model-admin

# Grant Alice access to the role
juju add-permission user-alice@canonical.com assignee role-model-admin

# Verify that Alice's access to the role has been granted successfully:
juju check-permission user-alice@canonical.com administrator model-bob@canonical.com/foo
```
````

(add-a-role)=
## Add a role

To add a new role to your JIMM controller, use the `add-role` command followed by the name you want to assign to the role. For example:

```text
juju add-role model-admin
```

> See more: {doc}`juju add-role <../reference/jaas-plugin>`

(view-all-the-current-roles)=
## View all the current roles

To view all the current roles, run the `list-roles` command. For example:

```text
juju list-roles [options]
```

> See more: {doc}`juju list-roles <../reference/jaas-plugin>`

(assign-a-user-to-a-role)=
## Assign a user to a role

To assign a user to a role, add an `assignee` permission between the user and the role. For example:

```text
juju add-permission user-alice@canonical.com assignee role-myrole
juju add-permission group-mygroup#member assignee role-myrole
juju add-permission user-everyone@external assignee role-my-role
```

> See more: {ref}`manage-permissions`

(rename-a-role)=
## Rename a role

To rename a role, run the `rename-role` command followed by the old name and the new name. For example:

```text
juju rename-role model-admin model-writer
```

> See more: {doc}`juju rename-role <../reference/jaas-plugin>`

(remove-a-role)=
## Remove a role

To remove a role from a JIMM controller, run the `remove-role` command followed by the name of the role. For example:

```text
juju remove-role model-admin
```

> See more: {doc}`juju remove-role <../reference/jaas-plugin>`
