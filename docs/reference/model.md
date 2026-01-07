(model)=
# Model
> See first: {external+juju:ref}`Juju | Model <model>`

(model-tag)=
## Model tag

A model tag has the following format:

```text
model-<controller name>/<model name>
```

where `<controller name>` specifies name of the controller on which the model
is running and `<model name>` specifies the name of the model.

(model-permission)=
## Model permission

A model permission describes what an entity can do on a model.

(list-of-model-permissions)=
### List of model permissions

(model-permission-reader)=
#### `reader`

Abilities: Can view the content of a model without changing it. Can use any of the read commands.

(model-permission-writer)=
#### `writer`

Abilities: Can deploy and manage applications on the model.

(model-permission-administrator)=
#### `administrator`

Abilities: Can do anything that it is possible to do at the level of a model.This grants permissions to all resources that inherit from model access.