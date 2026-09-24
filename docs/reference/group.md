---
myst:
  html_meta:
    description: "Complete reference for JAAS groups including group tags, member permissions, and managing collections of users."
---

(group)=
# Group
> See also: {ref}`manage-groups`

In JAAS, a group is a collection of users and/or groups.

A group is referenced by name (which is internally matched to a unique ID).

(group-tag)=
## Group tag

A group tag has the following format:

```text
group-<group id>
```

where `group id` represents the unique identifier of the group.

(group-permission)=
## Group permission

A group permission describes what an entity can do in a group.

(list-of-group-permissions)=
### List of group permissions

(group-permission-member)=
#### `member`

Abilities: Shares the group's access level to Juju resources and JIMM logs.

