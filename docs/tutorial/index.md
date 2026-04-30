---
myst:
  html_meta:
    description: "Learn how to deploy JIMM (the Juju Intelligent Model Manager) on MicroK8s with enterprise authentication and centralized Juju management."
---

(tutorial)=
# Get started with JAAS

In this tutorial we will be deploying JAAS -- that is, the Juju Intelligent Model Manager (JIMM) and all its dependencies -- on a local Kubernetes cloud, MicroK8s.

With JAAS set up, you will be able to enjoy enterprise-level authentication and authorization and the ability to view all of your Juju real estate from a single point of contact.

## Prerequisites

- A workstation, e.g., a laptop, that has sufficient resources to launch a virtual machine with 4 CPUs, 8 GB RAM, and 50 GB disk space.
- (Optional, recommended:) Some familiarity with Juju and the Terraform Provider for Juju.


## Set up an isolated test environment

For this tutorial we will set up an isolated environment using [Multipass](https://canonical.com/multipass). We will start by installing it, if not already installed:
```text
sudo snap install multipass --channel latest/stable
```

To create a `jaas-workshop` Multipass instance we'll use throughout this tutorial run:
```text
multipass launch 24.04 \
  --name jaas-workshop  \
  --cpus 4 \
  --memory 8G \
  --disk 50G \
  --timeout 1800 \
  --cloud-init https://raw.githubusercontent.com/canonical/multipass/refs/heads/main/data/cloud-init-yaml/cloud-init-charm-dev.yaml
```
This might take a while to run (~10min).

We will need the IP of the started Multipass instance later in the tutorial. To record the IP run:
```text
multipass list --format json | yq  '.list | .[] | select(.name == "jaas-workshop") | .ipv4[0]' > multipass_ip.txt
```
Then copy the file to the instance:
```text
multipass transfer multipass_ip.txt jaas-workshop:/home/ubuntu/multipass_ip.txt
```


Open a shell in the VM:
```text
multipass shell jaas-workshop
```

And load the instance IP we copied in a file earlier:
```text
multipass_ip=$(cat /home/ubuntu/multipass_ip.txt)
```


Make sure MicroK8s is correctly set up with the needed add-ons:
```text
# enable necessary add-ons
sudo microk8s enable host-access
# reconfigure metallb
sudo microk8s disable metallb
sudo microk8s enable metallb:10.64.140.43-10.64.140.49
```

Install the needed utilities we'll need in this tutorial:
```text
sudo snap install terraform --classic
sudo snap install vault
sudo snap install go --classic
sudo snap install jaas --channel 3/edge
```

You are now all set and ready to start deploying JAAS.

## Deploy the identity platform


JIMM uses OAuth 2.0, a provider agnostic way of handling authentication. In this tutorial we will combine it with the Canonical Identical Platform, which uses Ory Hydra to provide an OAuth server and Kratos for user management, respectively. To speed things up, we will deploy all these components through the ready-to-use Terraform plan:
```text
git clone https://github.com/canonical/iam-bundle-integration.git && cd iam-bundle-integration
git checkout v1.0.0
```

And use Terraform to deploy the Canonical Identity Platform.
```text
terraform -chdir=examples/tutorial init
terraform -chdir=examples/tutorial apply -auto-approve
```

This will deploy the Canonical Identity Platform in two models - `core` and `iam` models. Run
```text
juju switch iam
```

to switch to the `iam` model and watch the deployment by running:
```text
juju status --watch 1s
```
Use `CTRL-C` to exit.

Eventually all applications should reach an `active` state. We will create a user on Kratos in the next step.

For now running `apply` with Terraform will create a few application offers. To see the created offers run:
```text
juju find-offers
```
Which should output something like:
```text
Store     URL                          Access  Interfaces
microk8s  admin/core.send-ca-cert      admin   certificate_transfer:send-ca-cert
microk8s  admin/core.traefik-route     admin   traefik_route:traefik-route
microk8s  admin/iam.kratos-info-offer  admin   kratos_info:kratos-info
microk8s  admin/iam.oauth-offer        admin   oauth:oauth
microk8s  admin/core.certificates      admin   tls-certificates:certificates
microk8s  admin/core.postgresql        admin   postgresql_client:database
```

We will use `admin/iam.oauth-offer` and `admin/core.send-ca-cert` offers to connect to JIMM later in this tutorial.

### Create a user

**It is important that all applications are in an `active` state before you proceed as described above**

To create a user we will run the `create-admin-account` action on Kratos and then set a password for the created user. Run:
```text
juju switch iam
# disable MFA to avoid unnecessary steps
juju config kratos enforce_mfa=False
# create the user and get the identity-id
action_output=$(juju run kratos/0 create-admin-account email=admin@workshop password=test username=admin --format json)
identity_id=$(echo $action_output | yq '."kratos/0".results."identity-id"')
# create a password secret and get the secret-uri
password_secret=$(juju add-secret password-secret password=Pa55word)
# grant kratos access to the created secret
juju grant-secret password-secret kratos
juju run kratos/0 reset-password identity-id="$identity_id" password-secret-id="$password_secret"
```

We have now created a user that can login with the `admin@workshop` email and `Pa55word` as password.

### Expose the identity bundle to your host machine

At the end of this tutorial you'll need to log in via a web browser. Given that we're in a Multipass VM, for that to work we need to expose the identity bundle to our host machine.

Locate the IP of your Multipass instance by running `multipass list` on your host machine, if you have multiple IPs pick **the first one**. You can do this by running on your host machine (in a different terminal window, not when shelled into the `jaas-workshop` VM):
```text
multipass list --format json | yq  '.list | .[] | select(.name == "jaas-workshop") | .ipv4[0]'
```

Then we configure the `traefik-public` in the `core` model:
```text
juju switch core
juju config traefik-public external_hostname=<multipass-ip>
sudo microk8s.kubectl port-forward traefik-public-0 443:443 --namespace=core --address=$multipass_ip  > /dev/null 2>&1 &
```

Run the following on your host machine to test that you've successfully exposed the identity bundle:
```text
curl -k https://$multipass_ip/.well-known/jwks.json
```
You should get a JSON response showing JWKS.

## Deploy JIMM

Now we will deploy JIMM and its dependencies into a new model. Let's first explore however what JIMM's dependencies are and what they are used for.

- OpenFGA: The OpenFGA charm provides authorisation, defining who is allowed to access what.
- PostgreSQL: PostgreSQL is JIMM's database of choice and stores persistent state. This PostgreSQL instance is used by both JIMM and OpenFGA.
- Vault: The Vault charm is used for storing sensitive user secrets. JIMM can be configured to store data in plain-text in PostgreSQL but this is not recommended for a production environment.
- Ingress: There are various charms that provide ingress into a K8s cluster. JIMM supports [Traefik Ingress](https://charmhub.io/traefik-k8s) and [Nginx Ingress Integrator](https://charmhub.io/nginx-ingress-integrator), this tutorial will use the latter.

```{note}
In a production environment you may want to structure your deployment slightly differently. You might consider placing your database on a VM and performing a cross-model relation.
You might also consider deploying a central Vault and relating to it cross-model.
```

Let's begin by creating a new model for JIMM and deploying the necessary applications:
```text
juju add-model jimm
# The channel used for the JIMM charm is currently 3/edge.
# At a later date this will be promoted to the 3/stable channel.
juju deploy juju-jimm-k8s --channel=3/edge jimm
juju deploy openfga-k8s --channel=2.0/stable openfga
juju deploy postgresql-k8s --channel=14/stable postgresql
juju deploy vault-k8s --channel=1.15/beta vault
juju deploy traefik-k8s --channel=latest/stable --trust ingress
juju relate jimm:ingress ingress
juju relate jimm:openfga openfga
juju relate jimm:database postgresql
juju relate jimm:vault vault
juju relate openfga:database postgresql
juju trust postgresql --scope=cluster
```

At this point only OpenFGA and PostgreSQL should be in an active state.
JIMM, Vault and the ingress should all be in a blocked state. Next we will relate JIMM to the cross-model offers we created previously.
```text
juju relate jimm admin/iam.oauth-offer
juju relate jimm admin/core.send-ca-cert
```
Before we move on we will deploy our own self-signed-certificates operator in order to eventually use JIMM with HTTPS.
We are doing this step afterwards to avoid issues that occur when performing the relations before the ingress is ready.
```text
juju deploy self-signed-certificates jimm-cert
juju relate ingress:certificates jimm-cert:certificates
```

Now move onto the next step to initialise Vault.

## Initialise Vault

The Vault charm has documentation on how to initialise it [here](https://charmhub.io/vault-k8s/docs/h-getting-started). But an abridged version of the steps are provided here.

To communicate with the Vault server we now need to setup 3 environment variables:

- `VAULT_ADDR`
- `VAULT_TOKEN`
- `VAULT_CAPATH`

Run the following commands to setup the first two variables that will enable communication with Vault.
```text
export VAULT_ADDR=https://$(juju status vault/leader --format=yaml | yq '.applications.vault.address'):8200; echo "Vault address =" "$VAULT_ADDR"
cert_juju_secret_id=$(juju secrets --format=yaml | yq 'to_entries | .[] | select(.value.label == "self-signed-vault-ca-certificate") | .key'); echo "Vault ca-cert secret ID=$cert_juju_secret_id"
juju show-secret ${cert_juju_secret_id} --reveal --format=yaml | yq '.[].content.certificate' > vault.pem && echo "saved certificate contents to vault.pem"
export VAULT_CAPATH=$(pwd)/vault.pem; echo "Setting VAULT_CAPATH from" "$VAULT_CAPATH"
```

Verify that Vault is accessible.
```text
vault status
```

The output should resemble the following
```text
Key                Value
---                -----
Seal Type          shamir
Initialized        false
Sealed             true
Total Shares       0
Threshold          0
Unseal Progress    0/0
Unseal Nonce       n/a
Version            1.15.6
Build Date         n/a
Storage Type       raft
HA Enabled         true
```

Now you can create an unseal key. For this tutorial we will only use a single key but in a production environment you will want to require more than 1 key-share to unseal Vault.
Run the following command to unseal Vault and export the unseal token and root key.
```text
key_init=$(vault operator init -key-shares=1 -key-threshold=1); echo "$key_init"
export VAULT_TOKEN=$(echo "$key_init" | sed -n -e 's/.*Root Token: //p'); echo "RootToken = $VAULT_TOKEN"
export UNSEAL_KEY=$(echo "$key_init" | sed -n -e 's/.*Unseal Key 1: //p'); echo "UnsealKey = $UNSEAL_KEY"
vault operator unseal "$UNSEAL_KEY"
```

Authorize the charm to be able to interact with Vault to manage its operations.
```text
vault_secret_id=$(juju add-secret vault-token token="$VAULT_TOKEN")
juju grant-secret vault-token vault
juju run vault/leader authorize-charm secret-id="$vault_secret_id"
juju remove-secret "vault-token"
```

Now run `juju status` again and confirm your Vault unit is in an active state.

Finally, save the root token and unseal key for later use.

```{note}
The unseal key is especially important. If your PC is restarted or any of the vault pods are recreated, then Vault will become resealed and the unseal key will be needed again.
```

```text
echo $UNSEAL_KEY > vault_unseal_key.txt
echo $VAULT_TOKEN > vault_token.txt
```

We are now ready to move onto the next step.

## Configure JIMM

Nearing the end, we will configure JIMM. Here we will configure required config parameters with an explanation of what they do.

Run the following commands:
```text
# The UUID value is used internally to represent the JIMM controller in OpenFGA relations/tuples.
# Changes to the UUID value after deployment will likely result in broken permissions.
# Use a randomly generated UUID.
juju config jimm uuid=3f4d142b-732e-4e99-80e7-5899b7e67e59

# A private and public key for macaroon based authentication with Juju controllers.
keypair=$(go run github.com/go-macaroon-bakery/macaroon-bakery/cmd/bakery-keygen/v3@latest)
juju config jimm public-key=$(echo $keypair | yq .public )
juju config jimm private-key=$(echo $keypair | yq .private )
```

Now you need to amend the `/etc/hosts` file on the `jaas-workshop` VM to create a DNS record the deployed ingress.
To do so you need to locate the IP MetalLB assigned to your ingress by running `juju status` and locating the IP
in the description of the `ingress` application ("Serving at <IP>").

```
# Get the metallb IP assigned to ingress
ingress_status=$(juju status --format json | yq '.applications.ingress.units."ingress/0"."workload-status".message')
metallb_ip=$(echo $ingress_status | cut -d' ' -f3 | cut -d'/' -f3)

# Create a DNS entry for jimm in your multipass VM.
echo "$metallb_ip jimm.workshop" | sudo tee -a /etc/hosts
# The address to reach JIMM, this will configure ingress and is also used for OAuth flows/redirects.
juju config jimm dns-name=jimm.workshop
juju config ingress external_hostname=jimm.workshop
```

Optionally, if you have deployed Juju Dashboard, you can configure JIMM to enable browser flow for authentication:
```text
juju config jimm juju-dashboard-location="<juju-dashboard-url>"
```

```{note}
However, in absence of a Juju Dashboard, you can still enable OAuth browser authentication flow by setting this parameter to any valid URL. For example:

```text
juju config jimm juju-dashboard-location="http://jimm.workshop/auth/whoami"
```

At this point you can run `juju status` and you should observe JIMM is active.
Navigate to `http://jimm.workshop/.well-known/jwks.json` to verify your JIMM deployment.

Finally we will obtain the ca-certificate generated to ensure that we can connect to JIMM with HTTPS.
This is necessary for the Juju CLI to work properly
```text
juju run jimm-cert/0 get-ca-certificate --quiet | yq .ca-certificate | sudo tee /usr/local/share/ca-certificates/jimm-test.crt
sudo update-ca-certificates --fresh
```

Verify that you can securely connect to JIMM with the following command:
```text
curl https://jimm.workshop/jimm-jimm/.well-known/jwks.json
```

Verify that you can login to your new controller with the Juju CLI.
You should be presented with a message to login.
```text
juju login jimm.workshop:443/jimm-jimm -c jimm.workshop
# Please visit https://<multipass-ip>/iam-hydra/oauth2/device/verify and enter code <code> to log in.
```
Visit the link from your browser, fill the username and password you created on Kratos and you should see.
```text
Welcome, admin@workshop. You are now logged into "jimm.workshop".

There are no models available. You can add models with
"juju add-model", or you can ask an administrator or owner
of a model to grant access to that model with "juju grant".
```

## Using Your JAAS Deployment

### Add Juju controllers

To use the deployed JAAS we must first add a Juju controller to the deployed JIMM. This can be done in two ways: either we manually bootstrap a Juju controller with the required `login-token-refresh-url` config option and then add it to JIMM, or we use JIMM to bootstrap a new controller of a specific Juju version.

Microk8s is a Juju built-in cloud and as such cannot be used to bootstrap controller via JIMM. For the purpose of this tutorial, we will register microk8s as a `workshop-k8s` cloud. Run:
```text
microk8s config | juju add-k8s workshop-k8s --cluster-name microk8s-cluster --client
```

To manually add a Juju controller we first need to bootstrap it.
```text
juju bootstrap workshop-k8s controller-workshop-1 --config login-token-refresh-url=http://jimm-endpoints.jimm.svc.cluster.local:8080/.well-known/jwks.json
```
Then we switch to the JIMM controller and add the bootstrapped controller:
```text
juju switch jimm.workshop
juju jaas register-controller controller-workshop-1 --local --tls-hostname juju-apiserver
```

Or we can use JIMM to bootstrap a Juju controller:
```
juju jaas bootstrap workshop-k8s controller-workshop-2 3.6.9 --config login-token-refresh-url=http://jimm-endpoints.jimm.svc.cluster.local:8080/.well-known/jwks.json
```
which will bootstrap a Juju 3.6.9 controller and add it to JIMM. We override the `login-token-refresh-url` config option in the command above, because of our specific setup for this tutorial.

We now have two Juju controllers added to JIMM: controller-workshop-1 and controller-workshop-2, both in microk8s, which you can verify by running:
```text
juju jaas controllers
```

### Deploy applications using Terraform

In this tutorial we will use the Juju Terraform provider to deploy a simple application, Wordpress, and relate it to a database.

When using the Juju Terraform provider with JAAS we need to provider a service account that is used to authenticate. To create one run:
```text
juju switch microk8s:iam
service_account=$(juju run hydra/0 create-oauth-client --quiet --format yaml redirect-uris='[https://10.64.140.44/jimm-jimm/callback]' scope='[openid,profile,email,phone,offline_access,offline]' grant-types='[authorization_code,refresh_token,urn:ietf:params:oauth:grant-type:device_code,client_credentials]' token-endpoint-auth-method=client_secret_basic --format json)
```
And then:
```text
export TF_VAR_client_id=$(echo "$service_account" | yq '.["hydra/0"].results."client-id"')
export TF_VAR_client_secret=$(echo "$service_account" | yq '.["hydra/0"].results."client-secret"')
export TF_VAR_client_key_data=$(microk8s config | yq '.users[0].user."client-key-data"')
export TF_VAR_client_certificate_data=$(microk8s config | yq '.users[0].user."client-certificate-data"')
```
to create environment variables that will be used in the Terraform plan.

Next, create a Terraform plan by running:
```text
kdir tf; cd tf
cat <<EOF > main.tf
terraform {
  required_providers {
    juju = {
      source = "juju/juju"
    }
    http = {
      source  = "hashicorp/http"
      version = "~> 3.0"
    }
  }
}

variable "client_key_data" {
  type = string
  description = "client key data"
}

variable "client_certificate_data" {
  type = string
  description = "client certificate data"
}

variable "client_id" {
  type = string
  description = "client id"
}
variable "client_secret" {
  type = string
  description = "client id"
}

provider "juju" {
    controller_addresses="$metallb_ip:443/jimm-jimm"

    client_id=var.client_id
    client_secret=var.client_secret
}

resource "juju_credential" "credential" {
  name = "k8s-credential"
  cloud {
    name = "workshop-k8s"
  }

  auth_type = "certificate"
  attributes = {
    "client-certificate-data" = var.client_key_data
    "client-key-data"         = var.client_certificate_data
  }
}

resource "juju_model" "workshop_model_1" {
  name = "workshop-model-1"

  credential = juju_credential.credential.name

  cloud {
    name = "workshop-k8s"
  }
}

resource "juju_application" "wordpress" {
  model_uuid = juju_model.workshop_model_1.uuid
  name       = "wordpress"

  charm {
    name = "wordpress-k8s"
  }
}

resource "juju_application" "mysql" {
  model_uuid = juju_model.workshop_model_1.uuid
  name       = "mysql"
  trust      = true

  charm {
    name = "mysql-k8s"
  }
}

resource "juju_integration" "wordpress_mysql" {
  model_uuid = juju_model.workshop_model_1.uuid

  application {
    name     = juju_application.wordpress.name
    endpoint = "database"
  }

  application {
    name     = juju_application.mysql.name
    endpoint = "database"
  }

}
EOF
```

Run:
```text
terraform init
terraform apply -auto-approve
```
to apply the plan and deploy Wordpress.

Once Terraform applies the plan you can inspect the created model by running:
```text
juju switch $TF_VAR_client_id@serviceaccount/workshop-model-1
juju status
```

### Permission management

JAAS uses relationship-based access control (ReBAC) to manage permissions.
```{note}
You can read more about it here: {ref}`Authorization <jaas-authorization>`
```

In this section we will:

1. create a new user in the identity platform (`user1@workshop`);
2. create a group (`workshop-users`);
3. add the user to the group;
4. grant the group administrator access to the model created via Terraform;
5. verify that the user can access the model.

```{note}
The tutorial earlier created `admin@workshop` in Kratos. Here we create an additional user to demonstrate how access can be delegated and verified.
```

#### Create a user in Kratos

Switch to the `iam` model and create a new user identity.
We follow the same approach as in the earlier "Create a user" step: create the identity, store the password in a Juju secret, then reset the password using that secret.

```text
juju switch microk8s:iam

# create user user1@workshop
action_output=$(juju run kratos/0 create-admin-account email=user1@workshop password=test username=user1 --format json)
identity_id=$(echo $action_output | yq '."kratos/0".results."identity-id"')
password_secret=$(juju add-secret password-secret-user1 password=Pa55word)
juju grant-secret password-secret-user1 kratos
juju run kratos/0 reset-password identity-id="$identity_id" password-secret-id="$password_secret"
```

At this point `user1@workshop` can log in using the password `Pa55word`.

#### Create a group and add the user

Now switch back to the JIMM controller and create a group.

```text
juju switch jimm.workshop

# create group workshop-users
juju jaas add-group workshop-users

# add user1@workshop to the group workshop-users
juju jaas add-permission user-user1@workshop member group-workshop-users

# check if user1@workshop is a member of group workshop-users
juju jaas check-permission user-user1@workshop member group-workshop-users
```

If membership is configured correctly, the final command should report that access is allowed.

#### Grant the group access to the model

The Terraform plan created a model named `workshop-model-1` owned by the service account identified by `$TF_VAR_client_id`.
We will grant *administrator* access to all members of `workshop-users`.

```text
# give group workshop-users administrator access to the model created via Terraform
juju jaas add-permission group-workshop-users#member administrator model-$TF_VAR_client_id@serviceaccount/workshop-model-1

# check if group members have administrator access to the model
juju jaas check-permission group-workshop-users#member administrator model-$TF_VAR_client_id@serviceaccount/workshop-model-1
```

You should see output similar to:

```text
access check for group-workshop-users#member on resource model-$TF_VAR_client_id@serviceaccount/workshop-model-1 with role administrator is allowed
```

Because `user1@workshop` is a member of the group, the user should also have administrator access:

```text
juju jaas check-permission user-user1@workshop administrator model-$TF_VAR_client_id@serviceaccount/workshop-model-1
```

#### Log in as the delegated user

Finally, confirm the permission grant works end-to-end by logging in as `user1@workshop` and verifying the model is visible and accessible.

```text
juju logout
juju login

juju whoami
juju models
```

Your `juju whoami` output should show `User: user1@workshop` and you should see the model `$TF_VAR_client_id@serviceaccount/workshop-model-1`.

Switch into the model and check you can view workload state:

```text
juju switch $TF_VAR_client_id@serviceaccount/workshop-model-1
juju status
```

If everything is configured correctly, `juju status` should report the applications deployed earlier (e.g. Wordpress and MySQL).

### Cross-model queries

When you manage many models, it’s often useful to run queries *across all models* without switching into each one.
JAAS supports this with `juju jaas query-models`: you provide a `jq` query expression on the command line, and JAAS evaluates it across the models you’re allowed to access.
The command prints the matching results as JSON. In the examples below we pipe the output to `jq .` purely to pretty-print it (the actual query is the argument passed to `juju jaas query-models`).

In this section we will:

0. Log back in `admin@workshop`
1. create a second model (`workshop-model-2`) with Terraform;
2. run cross-model queries to answer operational questions.

#### Log back in `admin@workshop`
```text
juju logout
juju login
```

#### Create another model

To make the examples more interesting, create a second model and deploy a small charm into it.
Here the plan:

```text
mkdir tf1; cd tf1
cat <<EOF > main.tf
terraform {
  required_providers {
    juju = {
      source = "juju/juju"
    }
    http = {
      source  = "hashicorp/http"
      version = "~> 3.0"
    }
  }
}

variable "client_key_data" {
  type = string
  description = "client key data"
}

variable "client_certificate_data" {
  type = string
  description = "client certificate data"
}

variable "client_id" {
  type = string
  description = "client id"
}
variable "client_secret" {
  type = string
  description = "client id"
}

provider "juju" {
    controller_addresses="$metallb_ip:443/jimm-jimm"

    client_id=var.client_id
    client_secret=var.client_secret
}

resource "juju_credential" "credential" {
  name = "k8s-credential"
  cloud {
    name = "workshop-k8s"
  }

  auth_type = "certificate"
  attributes = {
    "client-certificate-data" = var.client_key_data
    "client-key-data"         = var.client_certificate_data
  }
}

resource "juju_model" "workshop_model_2" {
  name = "workshop-model-2"

  credential = juju_credential.credential.name

  cloud {
    name = "workshop-k8s"
  }
}

resource "juju_application" "hello" {
  model_uuid = juju_model.workshop_model_2.uuid
  name       = "hello-kubecon"
  charm {
    name = "hello-kubecon"
  }
}
EOF

terraform init
terraform apply -auto-approve
```

After the apply, you should see a new model in `juju models`.

#### Query across all models

`juju jaas query-models` sends the query expression to JAAS for evaluation.
You can optionally pipe the JSON output to `jq .` to render it nicely.

##### Example 1: Find units in an error state

This query returns all units whose workload status is `error` across *all* accessible models:

```text
# query for all units in error state
juju jaas query-models '.applications[].units | .[] | select(."workload-status".current=="error")' | jq .
```

##### Example 2: Check the revision of a charm across models

This query returns the charm revision for `mysql-k8s` anywhere it is deployed:

```text
# query for the revision of the mysql-k8s charm in all models
juju jaas query-models '.applications[] | select(.charm=="mysql-k8s") | ."charm-rev"' | jq .
```

### Audit logs

JIMM provides audit logging functionality, tracking all requests/responses into the system. This gives administrators of JIMM the ability to audit changes at a very granular level.

```{note}
You can read more about it here: {ref}`Audit Logs <audit-logs>`
```

All requests to controllers and models are logged and can enable an analysis into why the state of the underlying Juju estate has changed.

To see all audit logs run:
```text
juju jaas audit-events
```

### Internal migrations

JAAS can perform *internal migrations* of a model between registered controllers.

#### Migrate a model between controllers

In this workshop we registered two controllers, `controller-workshop-1` and `controller-workshop-2`.
To demonstrate model migration we will bootstrap a new controller as we currently do not expose a way to tell which controller is running a model. Run:
```text
juju jaas bootstrap workshop-k8s controller-workshop-3 3.6.12 --config login-token-refresh-url=http://jimm-endpoints.jimm.svc.cluster.local:8080/.well-known/jwks.json
```
to bootstrap controller `controller-workshop-3`.

Once the controller is bootstrapped and added to JIMM, which we can verify by running
```text
juju jaas controllers
```
we can initiate the model migration of the `workshop-model-1` to controller `controller-workshop-3.

Run:
```text
juju jaas migrate-internal controller-workshop-3 $TF_VAR_client_id@serviceaccount/workshop-model-1
```

After the migration completes, we must run:
```text
model_uuid=$(juju show-model $TF_VAR_client_id@serviceaccount/workshop-model-1 --format json | yq '."workshop-model-1"."model-uuid"')
juju jaas update-migrated-model controller-workshop-3 $model_uuid
```

Now switch back to the model and confirm it’s healthy:

```text
juju switch $TF_VAR_client_id@serviceaccount/workshop-model-1
juju status
```

## Common Issues

The following are some common issues that may arise especially after a reboot of your local machine.

### JIMM shows invalid certificate

Try `curl https://jimm.workshop/.well-known/jwks.json`, if you receive an SSL certificate error then it's likely that the K8s ingress is no longer
serving the correct TLS certificate. The following command can help verify this.
```text
openssl s_client -showcerts -servername jimm.workshop -connect jimm.workshop:443 < /dev/null
```

If the certificates CN (Common Name) is "Kubernetes Ingress Controller Fake Certificate" then the self-signed certificate is missing.
Run the following to fix the issue.
```text
juju switch jimm
juju remove-relation ingress jimm-cert
```

Wait for the relation to be removed by observing the output from `juju status --relations --watch 2s`.
```text
juju relate ingress jimm-cert
```

Try `curl` the server again the certificate issue should be resolved.

### JIMM is not serving requests

If JIMM is not responding to requests, run the following commands to check the logs.
```text
microk8s kubectl logs jimm-0 -n jimm -c jimm -f
```

This will present the server logs and debug further.

### JIMM can't communicate with the identity platform

If JIMM's logs show an error similar to the following,
```text
{"level":"error","ts":"2024-05-31T07:00:03.827Z","msg":"failed to create oidc provider","error":"Get \"https://iam.10.64.140.43.nip.io/iam-hydra/.well-known/openid-configuration\": tls: failed to verify certificate: x509: certificate is not valid for any names, but wanted to match iam.10.64.140.43.nip.io"}
{"level":"error","ts":"2024-05-31T07:00:03.827Z","msg":"failed to setup authentication service","error":"failed to create oidc provider"}
{"level":"error","ts":"2024-05-31T07:00:03.827Z","msg":"shutdown","error":"failed to setup authentication service"}
```

then it is likely that the IP address for the `traefik-public` service in the `core` model has changed.

Run the following to verify this,
```text
juju switch core
juju status
juju config traefik-public external_hostname
juju status --format yaml | yq .applications.traefik-public.address
```

If you have used the `nip.io` service to setup hostnames, you may find that the address and IP no longer match.

Update the `external_hostname` config of `traefik-public` to the correct hostname and update your approved redirect URIs/URLs in your identity provider.
Assuming use of the `nip.io` service, we can simply rerun the steps used previously.
```text
TRAEFIK_PUBLIC=$(juju status traefik-public --format yaml | yq .applications.traefik-public.address)
juju config traefik-public external_hostname="iam.$TRAEFIK_PUBLIC.nip.io"
```

Another issue that might occur is certificate validation failures in JIMM. To resolve those try ro run the following:
```text
juju switch microk8s:core
juju remove-relation self-signed-certificates traefik-public
juju relate self-signed-certificates traefik-public
```
to cause recreation of Traefik certificates.

## Cleanup

Since we used Multipass to create an isolated environment for this tutorial all you need is to remove the instance:
```text
multipass delete -p jaas-workshop
```
