JIMM Bundle
===========

This bundle deploys a highly-available JIMM system.

Prerequisits
------------

In order to deploy the bundle a number of configuration items need to
be prepared:

### Identity-Location

JIMM needs to know the location of the candid service that will provide
the identity service. Configure the `identity-location` parameter in
`local.yaml` to configure this.

### Controller-Admin

In order to add models to the controller users need to be in the
controller admin group. An appropriate group needs to be identified,
or created, in the customers identity provider and configured as the
`controller-admin` parameter in `local.yaml`. If this is not present
then no controllers can be added to the JAAS system.

### Controller UUID

The UUID of the JAAS controller needs to be configured. A suitable UUID
can be created using `uuidgen`.

### `LOCAL/agent-username`, `LOCAL/agent-private-key` & `LOCAL/agent-public-key`

An agent user needs to be created in candid for JIMM to use to query
user information. To create such an agent admin access to the candid
service is required, most commonly this would be through the candid CLI
using the admin agent created when deploying the candid service. A new
agent is created using a command like:

    CANDID_URL=https://candid.example.com candid -a admin.agent create-agent grouplist@candid

This will display a json file containing the username along with both
the public and private keys. Copy these values into the respective files
in LOCAL.

### TLS Certificates

Get appropriate certificates from your CA and store the certificate
chain in `LOCAL/jimm.crt`, and the private key in `LOCAL/jimm.key`.

Deployment
----------

The bundle has some deployment options. To deploy just the base bundle,
with all required secrets, run:

    juju deploy ./bundle.yaml --overlay local.yaml

If prometheus monitoring is also required in the model then run:

    juju deploy ./bundle.yaml --overlay local.yaml --overlay overlay-prometheus.yaml

Note that this command can be run on a previously deployed base system to
"upgrade" it to provide prometheus.
