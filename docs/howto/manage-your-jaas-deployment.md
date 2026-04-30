---
myst:
  html_meta:
    description: "Complete guide to deploying and managing JAAS with JIMM controller, OpenFGA, PostgreSQL, Vault on Kubernetes with authentication configuration."
---

(manage-your-jaas-deployment)=
# Manage your JAAS deployment

(deploy-JAAS)=
## Deploy JAAS

```{note}
In order to deploy JAAS and all its components you must use a Juju controller with a minimum version of 3.x.

In order to interact with JAAS as a user, you must use a Juju CLI with a minimum version of 3.5.4.

JAAS supports Juju controllers with a minimum version 3.4.

```


TBA (for now please see {doc}`the tutorial <../tutorial/index>`)

<!--
To deploy JAAS:

```{note}
While some of the core components (the JIMM controller and OpenFGA) must be deployed on a Kubernetes cloud, others (PostgreSQL and Vault) can also be deployed on a machine cloud and provided through cross-model relations. Note however that the core components of JAAS all require a Kubernetes cloud.
```

1. Deploy an authentication provider. A Juju-native way is the Canonical Identity Platform. Use a preexisting Juju controller to deplot it:

```text
juju add-model iam
juju deploy identity-platform --trust --channel 0.2/edge
juju offer hydra:oauth
juju offer self-signed-certificates:send-ca-cert
```

2. Set up external IdP.

3. Use a preexisting Juju controller to deploy the JIMM controller and all of its dependencies, including an external identity provider.

```text
juju add-model jimm
# The channel used for the JIMM charm is currently 3/edge.
# At a later date this will be promoted to the 3/stable channel.
juju deploy juju-jimm-k8s --channel=3/edge jimm
juju deploy openfga-k8s --channel=2.0/stable openfga
juju deploy postgresql-k8s --channel=14/stable postgresql
juju deploy vault-k8s --channel=1.15/beta vault
juju deploy nginx-ingress-integrator --channel=latest/stable --trust ingress
juju relate jimm:nginx-route ingress
juju relate jimm:openfga openfga
juju relate jimm:database postgresql
juju relate jimm:vault vault
juju relate openfga:database postgresql
juju relate jimm admin/iam.hydra
juju relate jimm admin/iam.self-signed-certificates
juju deploy self-signed-certificates jimm-cert
juju relate ingress jimm-cert
```

4. Initialize Vault.

Install the Vault client:

```text
sudo snap install vault
```

Set up the variables that will enable communication with Vault:

```
export VAULT_ADDR=https://$(juju status vault/leader --format=yaml | yq '.applications.vault.address'):8200; echo "Vault address =" "$VAULT_ADDR"
cert_juju_secret_id=$(juju secrets --format=yaml | yq 'to_entries | .[] | select(.value.label == "self-signed-vault-ca-certificate") | .key'); echo "Vault ca-cert secret ID =" "$cert_juju_secret_id"
juju show-secret ${cert_juju_secret_id} --reveal --format=yaml | yq '.[].content.certificate' > vault.pem && echo "saved certificate contents to vault.pem"
export VAULT_CAPATH=$(pwd)/vault.pem; echo "Setting VAULT_CAPATH from" "$VAULT_CAPATH"
```

Verify that Vault is accessible:

```
vault status
```

Create an unseal key:

```text
key_init=$(vault operator init -key-shares=1 -key-threshold=1); echo "$key_init"
export VAULT_TOKEN=$(echo "$key_init" | sed -n -e 's/.*Root Token: //p'); echo "RootToken = $VAULT_TOKEN"
export UNSEAL_KEY=$(echo "$key_init" | sed -n -e 's/.*Unseal Key 1: //p'); echo "UnsealKey = $UNSEAL_KEY"
vault operator unseal "$UNSEAL_KEY"
```

Authorize the charm to interact with Vault and manage its operations:

```text
vault_secret_id=$(juju add-secret vault-token token="$VAULT_TOKEN")
juju grant-secret vault-token vault
juju run vault/leader authorize-charm secret-id="$vault_secret_id"
juju remove-secret "vault-token"
```

Save the root token and unseal key for later use:

```text
echo $UNSEAL_KEY > vault_unseal_key.txt
echo $VAULT_TOKEN > vault_token.txt
```

5. Configure JIMM

```
# The UUID value is used internally to represent the JIMM controller in OpenFGA relations/tuples.
# Changes to the UUID value after deployment will likely result in broken permissions.
# Use a randomly generated UUID.
juju config jimm uuid=3f4d142b-732e-4e99-80e7-5899b7e67e59
# The address to reach JIMM, this will configure ingress and is also used for OAuth flows/redirects.
juju config jimm dns-name=test-jimm.localhost
# A private and public key for macaroon based authentication with Juju controllers.
juju config jimm public-key="<public-key>"
juju config jimm private-key="<private-key>"
# If you have deployed `juju-dashboard`:
juju config jimm juju-dashboard-location="<juju-dashboard-url>"
```
-->

## Create a JIMM controller admin

### Prerequisites

For this how-to you will need the following:

- A basic understanding of JAAS tags.
- A running JAAS environment, see {doc}`the tutorial <../tutorial/index>`.
- An understanding of Juju permissions, see the [Juju docs](https://juju.is/docs/juju/user-permissions).

### Creating an admin user

In order to create an initial admin user we must use the config option `controller-admins`.

The format for `controller-admins` is a space separated list of email addresses or service accounts. This means
that entries can be of the form `name@domain.com` or `client-id@serviceaccount`.

Run the following command replacing the contents with your email address to configure your user as a JIMM admin.

```text
juju config jimm controller-admins="username@domain.com"
```

```{tip}

See also: [Charmhub | juju-jimm-k8s > Configurations > controller-admin](https://charmhub.io/juju-jimm-k8s/configurations#controller-admins)

Now you can verify that you have admin access to JIMM.

Ensure you have the `jaas` plugin installed:

```text
sudo snap install jaas --channel=3/stable
```

The following commands are particularly useful for interacting with controllers.

```text
juju controllers --managed
juju list-audit-events
```

In a fresh setup, the first should return an empty list, showing that no controllers have been added to JIMM.

The second command returns a list of audited events that JIMM has recorded. More information on JIMM's audit log feature
is available at the following {ref}`audit-logs`.

(integrate-jaas-with-the-canonical-observability-stack)=
## Integrate JAAS with the Canonical Observability Stack

This document shows how to integrate the different components of JAAS with the
[Canonical Observability Stack][cos] to enable pre-configured dashboards and alerting rules.

The Canonical Observability Stack is a Juju bundle that includes a series of
open source observability applications and related automation.
For the complete list of components in COS, read the
[Stack variants](https://documentation.ubuntu.com/observability/latest/explanation/architecture/).

### Prerequisites

- A running `COS-Lite` bundle.
  You can follow the [COS tutorial](https://documentation.ubuntu.com/observability/latest/tutorial/) guide.
  tutorial to get you started. Make sure to follow the section **Deploy the COS Lite bundle with overlays** section to create offers.
- A running JAAS. Please refer to the deployment {doc}`the tutorial <../tutorial/index>`.

```{tip}
[Juju offers](https://juju.is/docs/juju/manage-offers) are a way of sharing software as a service between models. Make sure you deploy COS and setup offers so that you can relate to it across models.
```

It is generally recommended to keep the observability stack separate from any observed applications to separate failure domains.
This document assumes that JAAS and the COS bundle are deployed to different models.

This how-to assumes that Vault and PostgreSQL are deployed alongside JIMM and OpenFGA. Depending on your approach, this may not be true.
Additionally this how-to assumes the names of the deployed applications, which might differ in your environment.

### Integration approaches

There are 2 possible  integration approaches depending on your networking / deployment setup:

1. If you are able to send metrics and logs directly to the observability platform components follow
   the Integrate JAAS with COS-Lite section
2. If you prefer using a telemetry collector component follow
   the Integrate JAAS with COS-Lite through Grafana-Agent section

### Integrate JAAS with COS-Lite

#### Grafana integration

Assuming you deployed the COS-Lite bundle in model `cos-model` with user admin, use the following
commands to integrate the JAAS applications by means of an application offer.

```text
juju integrate jimm admin/cos-model.grafana-dashboards
juju integrate openfga admin/cos-model.grafana-dashboards
juju integrate postgresql admin/cos-model.grafana-dashboards
juju integrate vault admin/cos-model.grafana-dashboards
```

#### Loki integration

Assuming you deployed the COS-Lite bundle in model cos-model with user admin, use the following commands
to integrate JAAS by means of an application offer.

```text
juju integrate jimm admin/cos-model.loki-logging
juju integrate openfga admin/cos-model.loki-logging
juju integrate postgresql admin/cos-model.loki-logging
juju integrate vault admin/cos-model.loki-logging
```

#### Prometheus integration

Assuming you deployed the COS-Lite bundle in model `cos-model` with user admin, use the following commands to integrate JAAS by means of an application offer.

```text
juju integrate jimm admin/cos-model.prometheus-scrape
juju integrate openfga admin/cos-model.prometheus-scrape
juju integrate postgresql admin/cos-model.prometheus-scrape
juju integrate vault admin/cos-model.prometheus-scrape
```

### Integrate JAAS with COS-Lite through Grafana-Agent

You first need to deploy the [Grafana-Agent operator](https://charmhub.io/grafana-agent-k8s), which is a telemetry collector used
to aggregate and push information to the COS-lite bundle.

```{tip}
Note that you may perform some relations directly with the COS applications. E.g. the Grafana relation shares any dashboards from the charm to Grafana. This relation should be done as described in the previous section.
```

To deploy Grafana-Agent run:

```text
juju deploy grafana-agent-k8s --channel latest/stable --trust
```

#### Forward Prometheus metrics

Integrate Grafana-Agent with JAAS by running the following commands:

```text
juju integrate grafana-agent-k8s jimm:metrics-endpoint
juju integrate grafana-agent-k8s openfga:metrics-endpoint
juju integrate grafana-agent-k8s postgresql:metrics-endpoint
juju integrate grafana-agent-k8s vault:metrics-endpoint
```

#### Forward Loki metrics

Integrate Grafana-Agent with JAAS by running the following commands:

```text
juju integrate grafana-agent-k8s jimm:logging
juju integrate grafana-agent-k8s openfga:log-proxy
juju integrate grafana-agent-k8s postgresql:logging
juju integrate grafana-agent-k8s vault:logging
```

#### Integrate Grafana-Agent with COS-Lite

Assuming you deployed the COS-Lite bundle in model `cos-model` with user admin,
use this command to integrate the Grafana-Agent with Prometheus by means of an application offer.

```text
juju integrate grafana-agent-k8s admin/cos-model.prometheus-receive-remote-write
```

Assuming you deployed the COS-Lite bundle in model `cos-model` with user admin,
use this command to integrate the Grafana-Agent with Loki by means of an application offer.

```text
juju integrate grafana-agent-k8s admin/cos-model.loki-logging
```

### Access the dashboards

You can get the Grafana IP address with the [`juju status`](https://juju.is/docs/juju/status) command.
The default port for the Grafana HTTP server is 3000.

The default credentials are:

- **Username**: admin
- **Password**: you can get the password with the juju action [`get-admin-password`](https://charmhub.io/grafana-k8s/actions).

Once in, you will see a vertical menu bar on the left side of the page.
You will find the available alerts by clicking on the Alerting menu.
You will find the available dashboards by clicking on the Dashboards menu

[canonical]: https://canonical.com/
[iam]: https://charmhub.io/topics/canonical-identity-platform
[cos]: https://charmhub.io/topics/canonical-observability-stack


(equip-your-jaas-deployment-with-tls-ingress)=
## Equip your JAAS deployment with TLS ingress

The NGINX Ingress Integrator is a charm responsible for creating Kubernetes ingress rules,
these rules can be hardened via TLS and the charm provides a means to do so. See [here](https://charmhub.io/nginx-ingress-integrator).

Our LEGO charms provide certificates for charms from a desired ACME server and can be integrated
with the integrator to enable TLS at the ingress level. See [here](https://charmhub.io/httprequest-lego-k8s).

You will require a domain that your ACME is aware of and an NGINX ingress controller installed
on your Kubernetes cluster.

With JAAS deployed, you can deploy both LEGO and the integrator, and integrate your LEGO charm deployment
to your ingress integrator, and then the ingress integrator to JIMM to enable TLS ingress for your deployment.

(integrate-jaas-with-the-juju-dashboard)=
## Integrate JAAS with the Juju dashboard

Juju dashboard is a web UI that is intended to supplement the CLI experience with aggregate views and at a glance health checks.

This how-to provides you with instructions on how to setup Juju Dashboard for your JAAS deployment.

```{tip}
To explore Juju Dashboard features you can go [here](https://juju.is/docs/juju/the-juju-dashboard).
```

### Prerequisites

For this how-to you will need the following:

- A running JAAS environment, see {doc}`the tutorial <../tutorial/index>`.

### Deploy Juju Dashboard

First deploy the Juju Dashboard charm.

```text
juju switch <model_where_jimm_is>
juju deploy juju-dashboard-k8s dashboard
juju integrate dashboard jimm-app
```

Then you need to expose your dashboard through an ingress.

```{tip}
You can follow {ref}`equip-your-jaas-deployment-with-tls-ingress` to add TLS to your ingress.
```

```text
juju deploy nginx-ingress-integrator dashboard-ingress
juju integrate dashboard dashboard-ingress
juju config dashboard-ingress service-hostname="<https://hostname>""
```

You will visit your dashboard at `https://hostname`.

Now you need to configure JIMM to accept requests coming from `https://hostname`.

```text
juju config jimm-app cors-allowed-origins="https://hostname"
juju config jimm-app juju-dashboard-location="https://hostname"
```

Now go to `https://hostname`, sign in through the identity provider you setup during JAAS deployment, and you are in the dashboard.

(harden-your-deployment)=
## Harden your deployment

Configure JIMM to use CORS using the configuration option `cors-allowed-origins`.

> See more: [Charmhub | JIMM-K8S > Configurations > `cors-allowed-origins`](https://charmhub.io/juju-jimm-k8s/configurations#cors-allowed-origins)

Integrate JIMM with Self-Signed Certificates using the `receive-ca-cert` relation endpoint.

> See more: [Charmhub | JIMM-K8s > Integrations > `receive-ca-cert`](https://charmhub.io/juju-jimm-k8s/integrations)

Enable TLS for PostgreSQL.

> See more: [Charmhub | PostgreSQL K8s > Enable TLS](https://charmhub.io/postgresql-k8s/docs/t-enable-tls?channel=14/stable)