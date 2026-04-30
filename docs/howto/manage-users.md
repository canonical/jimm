---
myst:
  html_meta:
    description: "Learn how to set up new users in JAAS, configure DNS addresses, manage login credentials, and control user access to resources."
---

(manage-users)=
# Manage users
> See first: {ref}`user`

(add-a-user)=
## Add a user

Configure your JIMM controller to have a DNS address and share it with the user.

> See more: [Charmhub | `juju-jimm-k8s` > Configurations > `dns-name`](https://charmhub.io/juju-jimm-k8s/configurations#dns-name)

Add a Juju controller to JAAS.

Add the user to JAAS.

> See more: {ref}`control-user-access`

Assuming the user already has the `juju` CLI client installed, get the user to log in to JIMM using the DNS address. For example:

```text
juju login test-jimm.localhost:443 -c jaas
```

This will print a string that includes a link to the identity provider connected to the JIMM controller. The user can use this to log in.

Once that's done, if the user runs `juju controllers`, they should be able to see the `jaas` controller, and commands like `juju models` should work work. However, to perform any meaningful operations, the user will also need access to a cloud by adding a cloud (or being granted access to one via `juju grant-cloud`), adding cloud credentials, and using `juju update-credentials` with the `--controller jimm` flag to make their credentials available to JIMM (which will store them in Vault). For example, assuming the `localhost` cloud:

```text
juju update-credentials localhost --controller jimm
```

At this point the user can start doing whatever the permissions you've granted them allow them to do, for example, add a model and deploy applications to it.

(control-user-access)=
## Control user access

```{note}
This permissions mechanism supplements the Juju way of granting a user access to controllers, clouds, models, and offers, and can be used in addition to it. See more: [Juju | Manage a user's access level](https://canonical-juju.readthedocs-hosted.com/en/3.6/user/howto/manage-users/) or [Terraform Provider Juju | Manage a user's access level](https://canonical-terraform-provider-juju.readthedocs-hosted.com/latest/howto/manage-users/).
```

To control user access to another entity -- be it a role or a group, or a resource such as a controller, a cloud, a model, or an application offer -- add a permission between the user and the entity.

> See more: {ref}`manage-permissions`
