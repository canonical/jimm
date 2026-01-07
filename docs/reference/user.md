(user)=
# User
> See first: {external+juju:ref}`Juju | User <user>`
>
> See also: {ref}`manage-users`

(user-tag)=
## User tag

A user tag has the following format:

```text
user-<username>
```

where `username` uniquely identifies a user including the domain.

Alternatively you may specify users as below:

|||
|-|-|
|`user-everyone@external`| Picks every user.|
|`<group tag>#member`| Picks every user in the given group.|
|`<role tag>#assignee` | Picks every user with a certain role.|
