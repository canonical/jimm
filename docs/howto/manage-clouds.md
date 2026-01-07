(manage-clouds)=
# Manage clouds
> See first: {ref}`cloud`

(control-user-access-to-a-cloud)=
## Control user access to a cloud

To grant a (collection of) user(s) access to a cloud, add a `can_addmodel` or `administrator` permission between the user(s) and the cloud. For example:

For example:

```text
# Make Alice cloud admin:
juju add-permission user-alice@canonical.com administrator cloud-mycloud

# Let all users add models on the cloud:
juju add-permission user-everyone@external can_addmodel cloud-mycloud

```

> See more: {ref}`manage-permissions`
