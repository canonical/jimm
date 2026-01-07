(role)=
# Role

> See also: {ref}`manage-roles`

In JAAS, a role is a property of an entity that describes what they can do in a JIMM controller.

(role-tag)=
## Role tag

A role tag has the following format:

```text
role-<role name>
role-<role id>
```

where `role id` represents the unique identifier of the role.

(role-permission)=
## Role permission

A role permission describes an entity's relationship to a role.

(list-of-role-permissions)=
## List of role permissions

(role-permission-assignee)=
### `assignee`

Abilities: Shares the role's access level to Juju resources and JIMM logs.

