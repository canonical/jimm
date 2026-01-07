(tutorial)=
# Get started with JAAS

In this tutorial we will be deploying JAAS -- that is, the Juju Intelligent Model Manager (JIMM) and all its dependencies -- on a local Kubernetes cloud, MicroK8s.

With JAAS set up, you will be able to enjoy enterprise-level authentication and authorization and the ability to view all of your Juju real estate from a single point of contact.

## Prerequisites

- A workstation, e.g., a laptop, that has sufficient resources to launch a virtual machine with 4 CPUs, 8 GB RAM, and 50 GB disk space.


## Set up an isolated test environment

On your machine, install Multipass and use it to set up an Ubuntu virtual machine (VM) called `my-juju-vm`. This will provide all the necessary tools and configuration for the tutorial (a localhost machine cloud and Kubernetes cloud, Juju, etc.).

> See more: {external+juju:ref}`Juju | Set things up <set-things-up>`. Please follow the automatic path with the `charm-dev` blueprint.

```{note}
This document also contains a manual path, using which you can set things up without the Multipass VM or the `charm-dev` blueprint. However, please note that the manual path may yield slightly different results that may impact your experience of this tutorial.
For best results we strongly recommend the automatic path, or else suggest that you follow the manual path in a way that stays very close to [the definition of the charm-dev blueprint](https://github.com/canonical/multipass-blueprints/blob/e270a76093aad7b178ce0df5b7aa00e9dfd9b054/v1/charm-dev.yaml).

```

Open a shell in the VM:

```text
multipass shell my-juju-vm
```

Make sure MicroK8s is correctly set up:

```text
# enable necessary add-ons
sudo microk8s enable dns host-access
# reconfigure metallb
sudo microk8s disable metallb
sudo microk8s enable metallb:10.64.140.43-10.64.140.49
```

Then install some handy tools to query and extract info from json and yaml:

```text
sudo apt install jq
sudo snap install yq
```

You are now all set and ready to deploy JAAS.

## Deploy the identity bundle

For this tutorial we will use Canonical's identity bundle to provide authentication. JIMM uses OAuth 2.0, a provider agnostic way of handling authentication.
Although any compliant identity provider could be used with JIMM, we recommend the use Canonical's identity platform for the best compatibility.
Canonical's identity bundle uses Ory Hydra/Kratos to provide an OAuth server and user management, respectively.

Now we will create a Juju model for the identity platform and deploy the bundle.

```text
juju add-model iam
juju deploy identity-platform --trust --channel latest/edge
```

Watch the deployment by running:

```text
juju status --watch 1s
```

Eventually all application should reach an `active` state except for the `kratos-external-idp-integrator` application. This application allows you to connect your identity platform
to an external identity provider like Google, GitHub, Microsoft, etc. This is necessary because the identity provider only acts as an identity broker. A summary on how to set this up is
provided in the next step.

Now run the following commands to create offers that will be consumed in the next step.

```text
juju offer hydra:oauth
juju offer self-signed-certificates:send-ca-cert
```

Running `juju status` should now two offers that we will use from a different model in the next step.

### Create a user

```text
# disable MFA to avoid unnecessary steps
juju config kratos enforce_mfa=False
# create the user and get the identity-id
juju run kratos/0 create-admin-account email=test@example.com password=test username=admin
# reset the password to make it active
juju add-secret password-secret password=abc
juju grant-secret password-secret kratos
juju run kratos/0 reset-password identity-id=<identity-id> password-secret-id=<secret:id>
```

### Expose the identity bundle to your host machine

> If you chose not to use a Multipass VM, you can skip this step.

At the end of this tutorial you'll need to log in via a web browser. Given that we're in a Multipass VM, for that to work we need to expose the identity bundle to our host machine.

Locate the IP of your Multipass instance by running `multipass list` on your host machine, if you have multiple IPs pick the first one.
```text
juju config traefik-public external_hostname=<multipass-ip>
sudo microk8s.kubectl port-forward traefik-public-0 443:443 --namespace=iam --address=<multipass_ip> &
```

Run the following on your host machine to test that you've successfully exposed the identity bundle:
```text
curl -k https://<multipass-ip>/iam-hydra/health/ready
```
The response should be:
```
{"status":"ok"}
```

## Deploy JIMM

Now we will deploy JIMM and its dependencies into a new model. Let's first explore however what JIMM's dependencies are and what they are used for.

- OpenFGA: The OpenFGA charm provides authorisation, defining who is allowed to access what.
- PostgreSQL: PostgreSQL is JIMM's database of choice and stores persistent state. This PostgreSQL instance is used by both JIMM and OpenFGA.
- Vault: The Vault charm is used for storing sensitive user secrets. JIMM can be configured to store data in plain-text in PostgreSQL but this is not recommended for a production environment.
- Ingress: There are various charms that provide ingress into a K8s cluster. JIMM supports [Traefik Ingress](https://charmhub.io/traefik-k8s) and [Nginx Ingress Integrator](https://charmhub.io/nginx-ingress-integrator), this tutorial will use the latter.

```{note}
In a production environment you may want to structure your deployment slightly differently.You might consider placing your database on a VM and performing a cross-model relation.
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
juju relate jimm admin/iam.hydra
juju relate jimm admin/iam.self-signed-certificates
```
Before we move on we will deploy our own self-signed-certificates operator in order to eventually use JIMM with HTTPS.
We are doing this step afterwards to avoid issues that occur when performing the relations before the ingress is ready.

```text
juju deploy self-signed-certificates jimm-cert
juju relate ingress:certificates jimm-cert:certificates
```

Now move onto the next step to initialise Vault.

## Initialise Vault

The Vault charm has documentation on how to initialise it [here](https://charmhub.io/vault-k8s/docs/h-getting-started?channel=1.15/beta). But an abridged version of the steps are provided here.

Install the Vault CLI client.

```text
sudo snap install vault
```

To communicate with the Vault server we now need to setup 3 environment variables:

- `VAULT_ADDR`
- `VAULT_TOKEN`
- `VAULT_CAPATH`

Run the following commands to setup the first two variables that will enable communication with Vault.

```text

export VAULT_ADDR=https://$(juju status vault/leader --format=yaml | yq '.applications.vault.address'):8200; echo "Vault address =" "$VAULT_ADDR"
cert_juju_secret_id=$(juju secrets --format=yaml | yq 'to_entries | .[] | select(.value.label == "self-signed-vault-ca-certificate") | .key'); echo "Vault ca-cert secret ID =" "$cert_juju_secret_id"
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

Authorises the charm to be able to interact with Vault to manage its operations.

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
```

```text
sudo snap install go --classic
# A private and public key for macaroon based authentication with Juju controllers.
go run github.com/go-macaroon-bakery/macaroon-bakery/cmd/bakery-keygen/v3@latest
# extract the public and private keys from the response
juju config jimm public-key="<public-key>"
juju config jimm private-key="<private-key>"
```

Now you need to amend your `/etc/hosts` to create a DNS record for your ingress.
To do so you need to locate the IP MetalLB assigned to your ingress by running `juju status` and locating the IP
in the description of the `ingress` application ("Serving at <IP>").

```
echo "<ip> test-jimm.localhost" | sudo tee -a /etc/hosts
# The address to reach JIMM, this will configure ingress and is also used for OAuth flows/redirects.
juju config jimm dns-name=test-jimm.localhost
juju config ingress external_hostname=test-jimm.localhost
```

Optionally, if you have deployed Juju Dashboard, you can configure JIMM to enable browser flow for authentication:

```text
juju config jimm juju-dashboard-location="<juju-dashboard-url>"
```

```{note}
However, in absence of a Juju Dashboard, you can still enable OAuth browser authentication flow by setting this parameter to any valid URL. For example:

```text
juju config jimm juju-dashboard-location="http://test-jimm.localhost/auth/whoami"
```

At this point you can run `juju status` and you should observe JIMM is active.
Navigate to `http://test-jimm.localhost/debug/info` to verify your JIMM deployment.

Finally we will obtain the ca-certificate generated to ensure that we can connect to JIMM with HTTPS.
This is necessary for the Juju CLI to work properly

```text
juju run jimm-cert/0 get-ca-certificate --quiet | yq .ca-certificate | sudo tee /usr/local/share/ca-certificates/jimm-test.crt
sudo update-ca-certificates --fresh
```

Verify that you can securely connect to JIMM with the following command:

```text
curl https://test-jimm.localhost/jimm-jimm/debug/info
```

Verify that you can login to your new controller with the Juju CLI.
You should be presented with a message to login.

```text
juju login test-jimm.localhost:443/jimm-jimm -c jimm-k8s
# Please visit https://<multipass-ip>/iam-hydra/oauth2/device/verify and entercode <code> to log in.
```
Visit the link from your browser, fill the credentials you've created before and you should see.
```text
Welcome, test@example.com. You are now logged into "jimm-k8s".

There are no models available. You can add models with
"juju add-model", or you can ask an administrator or owner
of a model to grant access to that model with "juju grant".
```

## Using Your JIMM Deployment

Now that you have JIMM running you can browse our additional guides to setup an admin user, add controllers and migrate existing workloads.

> See more: {ref}`howtos`

## Common Issues

The following are some common issues that may arise especially after a reboot of your local machine.

### JIMM shows invalid certificate

Try `curl https://jimm-test.localhost/debug/info`, if you receive an SSL certificate error then it's likely that the K8s ingress is no longer
serving the correct TLS certificate. The following command can help verify this.

```text
openssl s_client -showcerts -servername test-jimm.localhost -connect test-jimm.localhost:443 < /dev/null
```

If the certificates CN (Common Name) is "Kubernetes Ingress Controller Fake Certificate" then the self-signed certificate is missing.
Run the following to fix the issue.

```text
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
microk8s kubectl exec -it -n jimm jimm-0 -c jimm -- /charm/bin/pebble logs
```

This will present the server logs and debug further.

### JIMM can't communicate with the identity platform

If JIMM's logs show an error similar to the following,

```text
{"level":"error","ts":"2024-05-31T07:00:03.827Z","msg":"failed to create oidc provider","error":"Get \"https://iam.10.64.140.43.nip.io/iam-hydra/.well-known/openid-configuration\": tls: failed to verify certificate: x509: certificate is not valid for any names, but wanted to match iam.10.64.140.43.nip.io"}
{"level":"error","ts":"2024-05-31T07:00:03.827Z","msg":"failed to setup authentication service","error":"failed to create oidc provider"}
{"level":"error","ts":"2024-05-31T07:00:03.827Z","msg":"shutdown","error":"failed to setup authentication service"}
```

then it is likely that the IP address for the `traefik-public` and `traefik-admin` services in the `iam` model have changed.

Run the following to verify this,

```text
juju switch iam
juju status
juju config traefik-public external_hostname
juju status --format yaml | yq .applications.traefik-public.address
```

If you have used the `nip.io` service to setup hostnames, you may find that the address and IP no longer match.

Update the `external_hostname` config of `traefik-public` to the correct hostname and update your approved redirect URIs/URLs in your identity provider.
Assuming use of the `nip.io` service, we can simply rerun the steps used previously.

```text
TRAEFIK_PUBLIC=$(juju status traefik-public --format yaml | yq .applicationstraefik-public.address)
juju config traefik-public external_hostname="iam.$TRAEFIK_PUBLIC.nip.io"
```

## Cleanup

To remove the Juju controller you initially created and all models with associated applications, run the following command:

```text
juju destroy-controller --destroy-all-models --destroy-storage --no-prompt jimm-demo-controller
```

And to cleanup the Multipass VM if one was used:

```text
multipass delete --purge jimm-deploy
```
