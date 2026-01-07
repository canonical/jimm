(cloud)=
# Cloud
> See first: {external+juju:ref}`Juju | Cloud <cloud>`


(cloud-tag)=
## Cloud tag

A cloud tag has the following format:

```text
cloud-<cloud name>
```

(cloud-permission)=
## Cloud permission

A cloud permission describes what an an entity can do on a cloud.

(list-of-cloud-permissions)=
### List of cloud permissions

(cloud-permission-administrator)=
#### `administrator`

Abilities: Can do anything that it is possible to do at the level of a cloud.

(cloud-permission-can-addmodel)=
#### `can_addmodel`

Abilities: Can add a model and grant another user model-level permissions.