---
myst:
  html_meta:
    description: "Learn how to create, deploy, and manage Juju models in JAAS with controller placement, access control, and model lifecycle operations."
---

(manage-models)=
# Manage models

(create-a-model)=
## Create a model

```{mermaid}
flowchart LR
    U["User </br> (limited controller access)"]
    subgraph Controllers
        C1["Controller A</br>(supports openstack)"]
        C2["Controller B</br>(supports openstack)"]
        C3["Controller C</br>(does NOT support openstack)"]
        C4["Controller D</br>(supports openstack)"]
    end
    U -. no access .-> C1
    U -- access --> C2
    U -- access --> C3
    U -. no access .-> C4
    classDef ok fill:#b3e6b3,stroke:#2d662d,stroke-width:1px;
    classDef no fill:#f2b3b3,stroke:#662d2d,stroke-width:1px;
    class C1,C2,C4 ok;
    class C3 no;
    %% Model placement result
    subgraph Result
        M[(my-model)]
    end
    C2 -- selected for model --> M
```
_A user trying to add a model on an OpenStack cloud and their access to various controllers._

Adding a model in JAAS requires permissions on a cloud as well as a controller. In the example above, multiple controllers (in green) support the `openstack` cloud while controller C (in red) does not. The user has access to controllers B and C. Based on these 2 factors the only valid placement is controller B.

To create a model:

1. Make sure you have the necessary permissions:

- `can_addmodel` permission on the target cloud.
- `can_addmodel` permission on one or more controllers that support that cloud.

Grant these permissions with,

```text
juju add-permission user-example@canonical.com can_addmodel cloud-openstack
juju add-permission user-example@canonical.com can_addmodel controller-ctrlA
```

> See more: {ref}`verify-a-permission`

2. Add the model using the `jaas add-model` command, optionally specifying a target controller:
For example:

```
# View all the controllers you have access to:
juju jaas list-controllers
# Add a model to any valid controller
juju add-model <modelname>
# Add your model to the controller of your choice:
juju jaas add-model --target-controller <mytargetcontroller> <modelname>
```

> See more: {ref}`command-jaas-add-model`

When you don't specify a controller:

- If you only have access to one controller, JIMM will automatically add the model to that controller.
- If you have access to multiple controllers, JIMM will randomly choose one for you, prioritizing controllers hosted within the cloud to reduce latency.


(migrate-a-model-to-jaas)=
## Migrate a model to JAAS

This section describes how to migrate a model to JAAS from an existing Juju controller.

### Prerequisites

- A standalone Juju controller with a model (optionally with a running application).
- A basic understanding of Juju model migrations, see the [docs](https://juju.is/docs/juju/manage-models).
- A running JAAS, see the {doc}`the tutorial <../tutorial/index>`.
- Administrator permissions for JAAS, see our {doc}`how-to <./manage-your-jaas-deployment>`.

### Changes caused by model migration

When migrating a model to JAAS, local users are replaced with external users.

What this means is clearer if we take a look at a model's fully qualified name using the command `juju show-model`.
The full model name is `<model-owner>/<model-name>` where `<model-owner>` represents a Juju user like "admin".
When we migrate a model to JAAS we will create a yaml file where we provide a mapping of local users to
external users (users that come from an identity provider). This is important for 2 reasons:

1. Across controllers, multiple models can exist with the same name. E.g. Controller A hosts model `admin/foo`
and controller B hosts `admin/foo`.
By changing the model owner during import, we avoid conflicts importing many models with
the same name, owned by the same user.

2. When a controller is connected to JAAS, all application-offers are authorised by JAAS - see our {doc}`authorization doc <../explanation/jaas-authorization>` for more details on how JAAS authorises access to resources. This impacts any
existing cross-model relations and the user mapping defines who can continue to access these offers.

Once the model migration is complete the model's full name will change, e.g. from `admin/myModel` to `external@domain.com/myModel`.
Any offers the model hosts will have a new offer URL, e.g. from `admin/db.mysql` to `external@domain/db.mysql`.

Existing consumers of these offers will continue to function but new integrations must use the new URL.

### 1. Create a new Juju controller

This is only necessary if you have a Juju controller that does not have the `login-token-refresh-url` config option set to point
to JIMM. Use the following command to check if your controller is already using JAAS.

```text
juju switch <controller-name>
juju controller-config login-token-refresh-url
```

If the value is empty, we must first bootstrap a new Juju controller.

In order to use models with JAAS, the models must be running on a Juju controller that is properly configured. The
necessary config values cannot be set after bootstrap time, so any existing models must be migrated to a new controller.

The process of creating a Juju controller that is properly configured is described in {ref}`add-a-juju-controller`.

Once a Juju controller configured to communicate with JAAS has been created, move onto the next step.

### 2. Create user mapping file

An example mapping is below:

```yaml
admin: my-user@canonical.com
alice: alice@canonical.com
```

The file must include entries for:

1. the existing model owner;
2. any users with model access;
3. any users with access to offers hosted within the model.

If any entries are missing, the migration process will return an error indicating the missing users.
The special string `everyone@external`, representing all users, should not be included in the mapping.

You can use the `juju show-model <model-name>` command to see the users that have access to
the model.
You can also use the `juju list-offers` command alongside `juju show-offer <offer-name>`
to see the users that have access to any offers hosted within the model.

Any users that you do not wish to map must still be included with a null value or empty
string in place of the external user. This indicates that you are intentionally skipping this
local user, for example:

```yaml
alice: alice@canonical.com
bob: null # or ""
```

The mapping is consulted when Juju relations are periodically validated.

I.e. if an offer was previously consumed by the local Juju user "alice", when JIMM validates the relation it
will map user "alice" to "alice@canonical.com" to authorise access to the offer.
Revoking access from "alice@canonical.com" will result in the relation encountering an error.

It may not be possible to know all users that have have consumed offers when you wish to migrate a model,
especially if the `everyone@external` user was granted consume access but, using
[juju show-offer](https://documentation.ubuntu.com/juju/3.6/howto/manage-offers/#view-an-offers-details)
will help you to see all users that currently have access to an offer.

With a user mapping created, we can move onto the next step.

### 3. Validate cloud-credentials

Next we must check that we have a valid cloud-credential for the incoming model.
Run `juju show-model` and look for the `credential` field which should resemble the below:
```yaml
  credential:
    name: lxd-creds
    owner: admin
    cloud: localhost
    validity-check: valid
```

The new model owner must have a cloud-credential with the same name and for the same cloud.

Using the credential details above as an example - if model `admin/foo` is being migrated
and the user mapping contains the row `admin: joe@canonical` then `joe@canonical` must have a
credential (`juju show-credentials --controller`) named `lxd-creds` for cloud `localhost` in JIMM.

If you do not see a matching cloud-credential, you can add one by following the instructions in [managing cloud-credentials](https://juju.is/docs/juju/manage-credentials).

### 4. Migrate desired models

Once you have identified which models to migrate, created a user mapping file and validated that
the new owner has a valid cloud-credential, we can begin the process of model migration.

We will assume a model called `admin/my-model` is currently hosted on a controller called `my-controller` and a controller
called `workload-lxd` is connected to JIMM, where JIMM is known to the CLI as `jimm`.

A user mapping file called `test-mapping.yaml` is created and placed in `~/snap/juju/common/` to avoid Snap permission issues.

```text
juju switch my-controller:admin/my-model
juju jaas migrate admin/my-model jimm --backing-controller=workload-lxd --user-mapping="/home/user/snap/juju/common/test-mapping.yaml"
juju status --watch 2s
# Wait for model migration to complete.
juju switch jimm
juju models
```

At this point we should see the model has been migrated. If the model migration fails, `juju debug-log` should contain more info
and the migration will be aborted, leaving the model on the original controller.

After a successful migration, it is now possible to grant other users access to the model.
See Juju documentation for [more info](https://juju.is/docs/juju/user-permissions).

(migrate-a-model-within-jaas)=
## Migrate a model within JAAS

This document briefly covers how to migrate a model between two controllers within JAAS.

The below is also useful if you want to move a model to a specific controller.

### Prerequisites

- A basic understanding of Juju model migrations, see the [docs](https://juju.is/docs/juju/manage-models).
- A running JAAS with with multiple controllers attached, see the {doc}`the tutorial <../tutorial/index>` for deploying JAAS.
- Administrator permissions for JAAS. See more: {ref}`add-a-juju-controller`.

Connecting multiple controllers to JAAS can be accomplished adding LXD controllers as described in {ref}`add-a-juju-controller`.

### 1. Identify the new controller

JIMM does not currently expose information about which underlying controller hosts a specific model.
This information is stored in JIMM's database but the controller info returned when running `juju show-model <model-name>`
is JIMM's UUID and name, hiding the underlying controller information.

The following command will show you all the controllers connected to JIMM.

```text
juju list-controllers --managed
```

Currently to identify where the model is hosted, you must have access to the controllers connected to JIMM and switch to
those controllers in turn, and run `juju models` until you identify the correct controller.

Identify the controller you want to migrate to, only the name is necessary.

### 2. Migrate your model

The following command will migrate a model named `my-model` to the desired controller, in this case called `my-controller`.

```text
MODEL_NAME=my-model
MODEL_UUID=$(juju show-model $MODEL_NAME --format yaml | yq .$MODEL_NAME.model-uuid)
juju jaas migrate my-controller $MODEL_UUID
```

This will start the model migration process. You can now monitor the progress of the migration with `juju status` and `juju debug-log`.

Once the model has been successfully migrated, run the following command to update JIMM with the new controller information for the model.

```text
juju update-migrated-model my-controller $MODEL_UUID
```

This will update JIMM's internal state to locate the model on the specified controller.

At this point you can run `juju status` to see the model info.

### 3. Handling failed migration

If the model migration fails, then no further user input is required and the model should continue to exist on the original controller.

To inspect the reason for failure, consult the output from `juju debug-log` and `juju status`.

(upgrade-a-model)=
## Upgrade a model

How you upgrade a model depends on whether you’d be crossing patch versions (e.g., v3.6.23 -> v3.6.24), minor version (e.g., v3.5 -> v3.6) or major versions (v3 -> v4).  

- To upgrade a model's patch version, use `juju upgrade-model`.  

> See more: {external+juju:ref}`upgrade-a-model`  

- To upgrade a model's major or minor version, you must migrate your model to a controller of the target version and upgrade the model to that version. In JAAS you can use `juju jaas upgrade-to` to perform both steps. For example:  

```  
# Get the model UUID:  
MODEL_NAME=my-model  
MODEL_UUID=$(juju show-model $MODEL_NAME --format yaml | yq .$MODEL_NAME.model-uuid)  

# Start the migrate + upgrade:  
MODEL_NAME=my-model  
MODEL_UUID=$(juju show-model $MODEL_NAME --format yaml | yq .$MODEL_NAME.model-uuid)  
juju jaas upgrade-to juju-3-6-controller $MODEL_UUID  

# Verify that the procedure has succeeded 
# (output should show the target controller's details):  
juju jaas show-model $MODEL_UUID --format yaml
```  


> See more: {external+juju:ref}`Juju | juju show-model <command-juju-show-model>`, {ref}`command-jaas-upgrade-to` 

### How the upgrade works

JIMM coordinates the upgrade as a background workflow:

1. JIMM starts one workflow per model that will includes retry logic for each step.
2. Each workflow migrates the model to the target controller and then upgrades it to the Juju version of the target controller.
3. JIMM records workflow progress so `jaas show-model` can report the current state.

During the migration phase, the model may be temporarily unavailable in the same way as a standard Juju model migration. If the migration or upgrade fails, inspect the status with `jaas show-model` and consult the controller logs with `juju debug-log`.

(control-user-access-to-a-model)=
## Control user access to a model

To grant a (collection of) user(s) access to a model, add a `reader`, `writer`, or `administrator` permission between the user(s) and the model.

> See more: {ref}`add-a-permission`
