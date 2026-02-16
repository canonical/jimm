(command-jaas-add-cloud)=
# jaas add-cloud

## Summary
Add cloud to specific controller in jimm

## Usage
```jaas add-cloud [options] <controller_name> <cloud_name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--cloud` |  | The path to the cloud's definition file. The cloud name must be present in the file. |
| `--force` | false | Forces the cloud to be added to the controller |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju add-cloud mycontroller mycloud
    juju add-cloud mycontroller mycloud --cloud=./cloud-definition.yaml


## Details

Adds the specified cloud to a specific controller on JIMM.

One can specify a cloud definition via a yaml file passed with the --cloud
flag. If the flag is missing, the command will assume the cloud definition
is already known and will error otherwise.


(command-jaas-add-group)=
# jaas add-group

## Summary
Add group to jimm.

## Usage
```jaas add-group [options] <name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju add-group


## Details

Adds a group.


(command-jaas-add-model)=
# jaas add-model

## Summary
Adds a model to a specific controller.

## Usage
```jaas add-model [options] <model name> [cloud|region|(cloud/region)]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--config` |  | Specify the path to a YAML model configuration file or individual configuration options (`--config config.yaml [--config key=value ...]`) |
| `--credential` |  | Specify the credential to be used by the model |
| `--no-switch` | false | Choose not to switch to the newly created model |
| `--owner` |  | Specify the user who will own the model, if not the current user |
| `--target-controller` |  | Target controller for the model |

## Examples

    juju [jaas] add-model mymodel mycloud --target-controller jaas-controller-1
    juju [jaas] add-model mymodel us-east-1 --target-controller jaas-controller-1
	juju [jaas] add-model mymodel aws/us-east-1 --target-controller jaas-controller-2 --credential mycred
    juju [jaas] add-model mymodel --target-controller jaas-controller-3 --config key=value


## Details
Adds a model to a specific controller.

This command creates a new hosted model on the specified controller.

(command-jaas-add-permission)=
# jaas add-permission

## Summary
Add relation to JIMM.

## Usage
```jaas add-permission [options] <object> <relation> <target_object>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-f` |  | file location of JSON encoded tuples |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju add-permission user-alice@canonical.com member group-mygroup
    juju add-permission group-MyTeam#member admin model-mymodel
    juju add-permission -f /path/to/file.yaml


## Details

Grants access to a resource.

This command works at a low-level and commands like 'juju grant'
should be preferred in most cases.

Permissions in JIMM consist of an object, a relation and a target object.
These are used to define access control between resources.

The object and target object must be of the form &lt;tag&gt;-&lt;objectname&gt; or &lt;tag&gt;-&lt;object-uuid&gt;
E.g. "user-Alice" or "controller-MyController"

Certain reserved tags exist to denote specific resource types:
- The user-everyone@external tag represents all users.
- The controller-jimm tag represents the JIMM controller itself.

-f    Read from a file where filename is the location of a JSON encoded file of the form:
    [
        {
            "object":"user-mike",
            "relation":"member",
            "target_object":"group-yellow"
        },
        {
            "object":"user-alice",
            "relation":"member",
            "target_object":"group-yellow"
        }
    ]

Certain constraints apply when creating/removing permissions, namely:
Resources may be one of:

    user tag                = "user-<name>"
    group tag               = "group-<name>"
	role tag 			    = "role-&lt;name&gt;"
    controller tag          = "controller-<name>"
    model tag               = "model-<name>"
	cloud tag			    = "cloud-&lt;name&gt;"
    application-offer tag   = "applicationoffer-<name>"

If target_object is a group, the relation can only be:

    member

If target_object is a role, the relation can only be:

	assignee

If target_object is a controller, the relation can be one of:

    audit_log_viewer (only relevent for the JIMM controller)
	can_addmodel
    administrator

If target_object is a model, the relation can be one of:

    reader
    writer
    administrator

If target_object is a cloud, the relation can be one of:

	administrator
	can_addmodel

If target_object is an application offer, the relation can be one of:

    reader
    consumer
    administrator

If the object is a group, a userset must be applied by adding #member as follows.
This will grant/revoke access to all users within TeamA:

    group-TeamA#member administrator controller-MyController

Similarly if the object is a role, a userset must be applied by adding #member as follows.

	role-Auditor#assignee audit_log_viewer controller-MyController


(command-jaas-add-role)=
# jaas add-role

## Summary
Add role to jimm.

## Usage
```jaas add-role [options] <role name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju add-role myrole


## Details

Adds a role.


(command-jaas-bootstrap)=
# jaas bootstrap

## Summary
Bootstrap a Juju controller via JIMM

## Usage
```jaas bootstrap [options] <cloud name>[/region] <controller name> <juju version>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--config` |  | Specify a configuration file, or one or more configuration options.     (`--config config.yaml [--config key=value ...])` |
| `--credential` |  | The name of the cloud credential to use for bootstrapping. Only required if more than one credential is available for the cloud. |
| `--detach` | false | If set, the command will start the bootstrap job and return immediately with the job ID, without waiting for the job to complete. |
| `--format` | json | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

	juju [jaas] bootstrap <cloud[/region]> <controller name> <controller version>
	juju [jaas] bootstrap mycloud/region mycontroller 3.6.8
	juju [jaas] bootstrap mycloud/region mycontroller 3.6.8 --config controller-service-type=loadbalancer


## Details

Requests the JIMM server to bootstrap a Juju controller.
The controller will be created asychronously on the specificed
cloud and region.

By default the command will wait for the bootstrap job to complete
while printing the job logs. Note that the logs will not follow the
--output flag and will always be printed to stdout. This can allow
you to send the initial output with the job ID to a file, while the
logs are streamed to stdout.

Use the --detach flag to start the bootstrap job and return immediately,
printing only the job ID, without waiting for the job to complete.

The final argument, version, denotes the Juju controller to be bootstrapped.

Config options for the bootstrap process can be specified via one or more
--config options. Each --config option can either be a path to a YAML file
containing config options, or a key=value pair. If multiple --config options
are specified, they will be merged together, with later options taking
precedence over earlier ones. Key=value pairs will take precedence over
file contents.

These config options must match the config options supported by the Juju CLI
for the version of Juju being bootstrapped. See the Juju documentation for
the version specified for the full list of supported bootstrap config
options.

Note that some config options may not be specified as they will automatically
be set.
These are:

- login-token-refresh-url

Bootstrapping to a k8s cluster requires that the service set up to handle
requests to the controller be accessible outside the cluster. Typically this
means a service type of LoadBalancer is needed, and Juju does create such a
service if it knows it is supported by the cluster. This is performed by
interrogating the cluster for a well known managed deployment such as microk8s,
GKE or EKS.

See the Juju bootstrap documentation for more details and how to configure
bootstrap for a Kubernetes cluster Juju does not recognise.

Note that JIMM will internally do the following:
- download the juju CLI matching the desired controller version
- bootstrap a new controller
- register the controller with JIMM


(command-jaas-bootstrap-status)=
# jaas bootstrap-status

**Aliases:** destroy-status

## Summary
Displays logs for a bootstrap/destroy job

## Usage
```jaas bootstrap-status [options] <job id>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-f` | false | follow the logs |

## Examples

    juju bootstrap-status <id>
    juju destroy-status <id>


## Details

Displays logs for a bootstrap or destroy-controller job.


(command-jaas-bootstrap-stop)=
# jaas bootstrap-stop

## Summary
Stop an in-progress bootstrap job

## Usage
```jaas bootstrap-stop [options] <job id>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |

## Examples

    juju bootstrap-stop <id>


## Details

Stop a bootstrap job.


(command-jaas-check-permission)=
# jaas check-permission

## Summary
Check access to a resource.

## Usage
```jaas check-permission [options] <object> <relation> <target_object>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju check-permission user-alice@canonical.com administrator controller-aws-controller-1


## Details

Verifies access to a resource.


(command-jaas-controllers)=
# jaas controllers

**Aliases:** list-controllers

## Summary
Lists all controllers known to JIMM.

## Usage
```jaas controllers [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju controllers
    juju controllers --format json


## Details

Displays controller information for all controllers known to JIMM.


(command-jaas-destroy-controller)=
# jaas destroy-controller

## Summary
Destroy a Juju controller via JIMM

## Usage
```jaas destroy-controller [options] <controller name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--detach` | false | If set, the command will start the destroy-controller job and return immediately with the job ID, without waiting for the job to complete. |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `--no-prompt` | false | If set, the command will not prompt the user for the controller name before proceeding |
| `-o`, `--output` |  | Specify an output file |

## Examples

	juju [jaas] destroy-controller <controller name>
	juju [jaas] destroy-controller mycontroller
	juju [jaas] destroy-controller mycontorller --no-prompt


## Details

Requests the JIMM server to destroy a Juju controller.
The controller will be destroyed asynchronously.

By default the command will wait for the destroy-controller job to complete
while printing the job logs. Note that the logs will not follow the
--output flag and will always be printed to stdout. This can allow
you to send the initial output with the job ID to a file, while the
logs are streamed to stdout.

Use the --detach flag to start the bootstrap job and return immediately,
printing only the job ID, without waiting for the job to complete.

The argument denotes the name of the Juju controller to be destroyed.

Note that JIMM will internally do the following:
- download the juju CLI matching the controller version
- destroy the controller
- unregister the controller from JIMM

Destroying controllers on k8s clouds will only work on uju 3.6.10 or newer.
As a workaround, you can first unregister the controller and then destroy
it separately.


(command-jaas-documentation)=
# jaas documentation

## Summary
Generate the documentation for all commands

## Usage
```jaas documentation [options] --out <target-folder> --no-index --split --url <base-url> --discourse-ids <filepath>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--discourse-ids` |  | File containing a mapping of commands and their discourse ids |
| `--no-index` | false | Do not generate the commands index |
| `--out` |  | Documentation output folder if not set the result is displayed using the standard output |
| `--split` | false | Generate a separate Markdown file for each command |
| `--url` |  | Documentation host URL |

## Examples

    juju documentation
    juju documentation --split 
    juju documentation --split --no-index --out /tmp/docs

To render markdown documentation using a list of existing
commands, you can use a file with the following syntax

    command1: id1
    command2: id2
    commandN: idN

For example:

    add-cloud: 1183
    add-secret: 1284
    remove-cloud: 4344

Then, the urls will be populated using the ids indicated
in the file above.

    juju documentation --split --no-index --out /tmp/docs --discourse-ids /tmp/docs/myids


## Details

This command generates a markdown formatted document with all the commands, their descriptions, arguments, and examples.


(command-jaas-grant-audit-log)=
# jaas grant-audit-log

## Summary
Grants access to audit logs.

## Usage
```jaas grant-audit-log [options] <username>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |

## Examples

    juju grant-audit-log <username>


## Details

Grants a user access to read audit logs.


(command-jaas-help)=
# jaas help

## Summary
Show help on a command or other topic.

## Usage
```jaas help [flags] [topic]```

## Details

See also: topics


(command-jaas-import-model)=
# jaas import-model

**Aliases:** register-model

## Summary
Import a model to jimm.

## Usage
```jaas import-model [options] <controller name> <model uuid>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--owner` |  | switch the model owner to the desired user |

## Examples

    juju import-model mycontroller ac30d6ae-0bed-4398-bba7-75d49e39f189
    juju import-model mycontroller ac30d6ae-0bed-4398-bba7-75d49e39f189 --owner user@canonical.com


## Details

Imports a model running on a controller into JIMM's state.

When importing, it is necessary for JIMM to contain a set of cloud credentials
that represent a user's access to the incoming model's cloud.

The --owner command is necessary when importing a model created by a
local user and it will switch the model owner to the desired external user.


(command-jaas-list-audit-events)=
# jaas list-audit-events

**Aliases:** audit-events

## Summary
Displays audit events

## Usage
```jaas list-audit-events [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--after` |  | display events that happened after a specified time, formatted as RFC3339 |
| `--before` |  | display events that happened before specified time, formatted as RFC3339 |
| `--format` | yaml | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `--limit` | 0 | limit the maximum number of returned audit events |
| `--method` |  | display events for a specific method call |
| `--model` |  | display events for a specific model (model name is controller/model) |
| `-o`, `--output` |  | Specify an output file |
| `--offset` | 0 | offset the set of returned audit events |
| `--reverse` | false | reverse the order of logs, showing the most recent first |
| `--user-tag` |  | display events performed by authenticated user |

## Examples

    juju list-audit-events --after 2020-01-01T15:00:00 --before 2020-01-01T15:00:00 --user-tag user@canonical.com --limit 50
    juju list-audit-events --method CreateModel
    juju audit-events --after 2020-01-01T15:00:00 --format yaml


## Details

Returns audit log events.


(command-jaas-list-groups)=
# jaas list-groups

**Aliases:** groups

## Summary
List all groups.

## Usage
```jaas list-groups [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `--limit` | 0 | The maximum number of groups to return |
| `-o`, `--output` |  | Specify an output file |
| `--offset` | 0 | The offset to use when requesting groups |

## Examples

    juju list-groups


## Details

Lists all groups.


(command-jaas-list-migration-targets)=
# jaas list-migration-targets

## Summary
List migration targets for internal model migration.

## Usage
```jaas list-migration-targets [options] <model uuid>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

	juju list-migration-targets bb933db6-b151-417f-9a62-e3e80b4ebc9b


## Details

Requests a list of controllers connected to JIMM that are valid migration
targets for the specified model.

This is useful to obtain a filtered list of controllers that meets the following
criteria:
- The controller is not the current controller for the model.
- The controller can deploy to the the same cloud/region as the current controller.
- The controller is running a compatible Juju version i.e. newer than or equal to
  the current controller.


(command-jaas-list-permissions)=
# jaas list-permissions

**Aliases:** permissions

## Summary
List relations.

## Usage
```jaas list-permissions [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--object` |  | relation object |
| `--relation` |  | relation name |
| `--resolve` | true | resolves UUIDs to human readable tags |
| `--target` |  | relation target object |

## Examples

List all permissions

    juju list-permissions

List permissions where the target object match

    juju list-permissions --target model-mymodel

List permissions where the target object and relation match

    juju list-permissions --target model-mymodel  --relation admin


## Details

List permissions known to JIMM. Using the "target", "relation" and "object" flags,
only those permissions matching the filter will be returned.


(command-jaas-list-roles)=
# jaas list-roles

**Aliases:** roles

## Summary
List all roles.

## Usage
```jaas list-roles [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `--limit` | 0 | The maximum number of roles to return |
| `-o`, `--output` |  | Specify an output file |
| `--offset` | 0 | The offset to use when requesting roles |

## Examples

    juju list-roles list


## Details

Lists all roles.


(command-jaas-migrate)=
# jaas migrate

## Summary
Migrate models to JAAS, targetting the desired managed controller.

## Usage
```jaas migrate [options] <model-name> <jaas-name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--backing-controller` |  | Specify the name of the controller that will host the model in JIMM. |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--user-mapping` |  | Specify a comma-separated user mapping of local users to external users |

## Examples

    juju migrate alice/my-model my-jaas --backing-controller=controller-1 --user-mapping=./user-mapping.yaml


## Details

The migrate command migrates a model to JIMM.

This command is useful to take a model that is already running on a Juju controller
and migrate it to JIMM. During this process JIMM will modify the details of the model
to remove any local users with access to the model and replace the model owner with
an external user i.e. from alice -&gt; alice@canonical.com.

In order to determine the new model owner and to handle any existing application-offers
that have already been consumed with local users, you must specify a user mapping file
with the --user-mapping flag. This should point to a yaml file with a mapping of local
users to external users.
For example:

my-user-mapping.yaml:
'''
alice: alice@canonical.com
bob: bob@canonical.com
'''

The mapping must contain entries for all users that have access to the model and any offers
hosted within that model.
You can use the "juju show-model &lt;model-name&gt;" command to see the users that have access to
the model.
You can also use the "juju list-offers" command alongside "juju show-offer &lt;offer-name&gt;"
to see the users that have access to each offer.

Any users that you do not wish to be mapped must still be included with a null value or empty
string in place of the external user. This indicates that you are intentionally skipping this
local user, for example:
'''
alice: alice@canonical.com
bob: null # or ""
'''

The user mapping is consulted when relations are periodically validated. I.e. if an offer
was consumed by user "alice", when JIMM validates the relation it will understand that user
"alice" has been mapped and checks that "alice@canonical" still has access to the offer.
Revoking access from "alice@canonical.com" will result in the relation encountering an error.

It may not be possible to know all users that have have consumed offers from a model, but using
"juju show-offer &lt;offer-name&gt; --format yaml" you can see all users that have access to the
offer. This list should help determine which users to map in the user mapping file.

Any tools/scripts that refer to models by their full name (owner/name) will need to be
updated after migration to use the new external username or refer to models by their UUID.


(command-jaas-migrate-internal)=
# jaas migrate-internal

## Summary
migrate models to another controller within JAAS

## Usage
```jaas migrate-internal [options] <controller name> <model uuid> [<model uuid>...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju migrate-internal mycontroller 2cb433a6-04eb-4ec4-9567-90426d20a004 fd469983-27c2-423b-bebf-84f616fb036b ...
    juju migrate-internal mycontroller user@domain.com/model-a user@domain.com/model-b ...
    juju migrate-internal mycontroller user@domain.com/model-a fd469983-27c2-423b-bebf-84f616fb036b ...



## Details

The migrate-internal command migrates a model(s) between two controllers
in your JAAS system. This performs a model migration, but is named
"migrate-internal" to avoid confusion with the "migrate" command which migrates
a model to JAAS.

You may specify a model name (of the form owner/name) or model UUID.



(command-jaas-model-status)=
# jaas model-status

## Summary
Displays full model status

## Usage
```jaas model-status [options] <model uuid>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju model-status 2cb433a6-04eb-4ec4-9567-90426d20a004
    juju model-status 2cb433a6-04eb-4ec4-9567-90426d20a004 --format yaml


## Details

Displays full model status.


(command-jaas-purge-audit-logs)=
# jaas purge-audit-logs

## Summary
purge audit logs from the database before the given date

## Usage
```jaas purge-audit-logs [options] <date>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju purge-audit-logs 2021-02-03
    juju purge-audit-logs 2021-02-03T00
    juju purge-audit-logs 2021-02-03T15:04:05Z


## Details

Purges logs from the database before the given date.

The provided date must be formatted as an ISO8601 date string.


(command-jaas-query-models)=
# jaas query-models

## Summary
Query model statuses

## Usage
```jaas query-models [options] <query>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | json | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju query-models '.applications | with_entries(select(.key=="nginx-ingress-integrator"))'


## Details

Queries all models available to the current user performing the
query against each model status individually, returning the
collated query responses for each model.

The query runs against the output of "juju status --format json",
as such you can format your query against an output like this.

The queries expect a JQ query string.


(command-jaas-register-controller)=
# jaas register-controller

## Summary
Add controller to jimm

## Usage
```jaas register-controller [options] <filepath>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--dry-run` | false | Dry-run enabled will only print the controller details. |
| `--file` |  | Specify a file-path for controller details, use '-' to read from stdin. |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `--local` | false | If local flag is specified, then the local API addresses and CA cert of the controller will be used. |
| `-o`, `--output` |  | Specify an output file |
| `--public-address` |  | Specify a custom public address to use for dialing the controller. |
| `--tls-hostname` |  | Specify the hostname for TLS verification. |

## Examples

    juju register-controller mycontroller
    juju register-controller mycontroller --local


## Details

Registers a controller with JIMM.

Using the controller name provided, this command will inspect your
Juju client store for details on the specified controller.

Note that by default, this command assumes the controller has the public-hostname
field set, which will define the preferred address JIMM will use to contact the
controller. Use of a public address will also ignore any custom CA cert in your
local client store and assumes the server is secured with a public certificate.

Use the --local flag if the server is not configured with a public address or to
ignore the controller's public-hostname and use the custom CA of the controller.

A yaml formatted file can also be used as input for cases where the controller
is not available on the client. Using the --file will validate that the provided
controller name matches the name in the yaml file.
Using --file will ignore other flags like --public-address and --local.

Use the --dry-run flag to generate a sample file without registering the controller.
This can be used later as input to register-controller.


(command-jaas-remove-cloud)=
# jaas remove-cloud

## Summary
Remove cloud from specific controller in jimm

## Usage
```jaas remove-cloud [options] <controller_name> <cloud_name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju remove-cloud mycontroller mycloud


## Details

Removes the specified cloud from the specified controller in JIMM.


(command-jaas-remove-group)=
# jaas remove-group

## Summary
Remove a group.

## Usage
```jaas remove-group [options] <name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--force` | false | delete group without prompt |
| `--format` | smart | Specify output format (smart) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju remove-group mygroup


## Details

Removes a group.


(command-jaas-remove-permission)=
# jaas remove-permission

## Summary
Remove relation from JIMM.

## Usage
```jaas remove-permission [options] <object> <relation> <target_object>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-f` |  | file location of JSON encoded tuples |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju remove-permission user-alice@canonical.com member group-mygroup
    juju remove-permission group-MyTeam#member admin model-mymodel
    juju remove-permission -f /path/to/file.yaml


## Details

Revokes access to a resource.

This command works at a low-level and commands like 'juju grant'
should be preferred in most cases.

Permissions in JIMM consist of an object, a relation and a target object.
These are used to define access control between resources.

The object and target object must be of the form &lt;tag&gt;-&lt;objectname&gt; or &lt;tag&gt;-&lt;object-uuid&gt;
E.g. "user-Alice" or "controller-MyController"

Certain reserved tags exist to denote specific resource types:
- The user-everyone@external tag represents all users.
- The controller-jimm tag represents the JIMM controller itself.

-f    Read from a file where filename is the location of a JSON encoded file of the form:
    [
        {
            "object":"user-mike",
            "relation":"member",
            "target_object":"group-yellow"
        },
        {
            "object":"user-alice",
            "relation":"member",
            "target_object":"group-yellow"
        }
    ]

Certain constraints apply when creating/removing permissions, namely:
Resources may be one of:

    user tag                = "user-<name>"
    group tag               = "group-<name>"
	role tag 			    = "role-&lt;name&gt;"
    controller tag          = "controller-<name>"
    model tag               = "model-<name>"
	cloud tag			    = "cloud-&lt;name&gt;"
    application-offer tag   = "applicationoffer-<name>"

If target_object is a group, the relation can only be:

    member

If target_object is a role, the relation can only be:

	assignee

If target_object is a controller, the relation can be one of:

    audit_log_viewer (only relevent for the JIMM controller)
	can_addmodel
    administrator

If target_object is a model, the relation can be one of:

    reader
    writer
    administrator

If target_object is a cloud, the relation can be one of:

	administrator
	can_addmodel

If target_object is an application offer, the relation can be one of:

    reader
    consumer
    administrator

If the object is a group, a userset must be applied by adding #member as follows.
This will grant/revoke access to all users within TeamA:

    group-TeamA#member administrator controller-MyController

Similarly if the object is a role, a userset must be applied by adding #member as follows.

	role-Auditor#assignee audit_log_viewer controller-MyController


(command-jaas-remove-role)=
# jaas remove-role

## Summary
Remove a role.

## Usage
```jaas remove-role [options] <role name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | smart | Specify output format (smart) |
| `-o`, `--output` |  | Specify an output file |
| `-y` | false | delete role without prompt |

## Examples

    juju remove-role remove myrole


## Details

Removes a role.


(command-jaas-rename-group)=
# jaas rename-group

## Summary
Rename a group.

## Usage
```jaas rename-group [options] <name> <new name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |

## Examples

    juju rename-group mygroup newgroup


## Details

Renames a group.


(command-jaas-rename-role)=
# jaas rename-role

## Summary
Rename a role.

## Usage
```jaas rename-role [options] <role name> <new role name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |

## Examples

    juju rename-role myrole newrolename


## Details

Renames a role.


(command-jaas-revoke-audit-log)=
# jaas revoke-audit-log

## Summary
revokes access to audit logs.

## Usage
```jaas revoke-audit-log [options] <user>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |

## Examples

    juju revoke-audit-log user@canonical.com


## Details

Revokes user access to audit logs.


(command-jaas-set-controller-deprecated)=
# jaas set-controller-deprecated

## Summary
Sets controller deprecated status.

## Usage
```jaas set-controller-deprecated [options] <controller name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju set-controller-deprecated mycontroller


## Details

Sets the deprecated status of a controller.


(command-jaas-show-model)=
# jaas show-model

## Summary
Displays information about a model and its controller

## Usage
```jaas show-model [options] <model>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    jaas show-model 2cb433a6-04eb-4ec4-9567-90426d20a004
    jaas show-model alice@canonical.com/my-model
    jaas show-model alice@canonical.com/my-model --format json
    jaas show-model alice@canonical.com/my-model --format yaml


## Details

Displays information about which controller the specified model is running on.

The model can be specified using either:
  - Model UUID (e.g., "2cb433a6-04eb-4ec4-9567-90426d20a004")
  - Owner and model name (e.g., "alice@canonical.com/my-model")

The output includes the model name, model UUID, controller name, and controller UUID.


(command-jaas-unregister-controller)=
# jaas unregister-controller

## Summary
Remove controller from jimm

## Usage
```jaas unregister-controller [options] <name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--force` | false | force unregister a controller |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju unregister-controller mycontroller
    juju unregister-controller mycontroller --force


## Details

Deregisters a controller from JIMM.


(command-jaas-update-migrated-model)=
# jaas update-migrated-model

## Summary
Update the controller running a model.

## Usage
```jaas update-migrated-model [options] <controller name> <model uuid>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |

## Examples

    juju update-migrated-model mycontroller e0bf3abf-7029-4e48-9c26-68a7b6e02947


## Details

Updates a model known to JIMM that has been migrated
externally to a different JAAS controller.


(command-jaas-upgrade-to)=
# jaas upgrade-to

## Summary
Upgrades a controller to a specified version

## Usage
```jaas upgrade-to [options] <version> <model-uuid>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju upgrade-to 3.6.11 2cb433a6-04eb-4ec4-9567-90426d20a004


## Details

Upgrades a controller to a specified version.


