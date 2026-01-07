(manage-juju-controllers)=
# Manage Juju controllers
> Who: JIMM controller admin
>
> See also: {ref}`controller`

<!--
ADD:
juju register-controller
juju controllers --managed
add-cloud-to-controller
juju remove-cloud --target-controller
juju remove-controller
juju set-controller-deprecated
-->

(add-a-juju-controller)=
## Add a Juju controller

JIMM gives a centralised view of all models in the system. However the work of managing
the models is delegated to a set of Juju controllers deployed in various clouds
and regions.

These Juju controllers must be deployed with some specific options to ensure they work
correctly in the JAAS system. This document discusses how to bootstrap a Juju controller
such that it will work correctly in a JAAS system.

In this how-to we will show how to add Juju controllers deployed in both MicroK8s and LXD to
a JIMM controller.

### Prerequisites

For this how-to you will need the following:

- Basic knowledge of Juju
- A JIMM controller deployed in MicroK8s, see {doc}`the tutorial <../tutorial/index>`.
<!--- Administrator permission on the JIMM controller, see {ref}`add-a-juju-controller`.-->


### Prelude

There are 2 ways to add controllers to JAAS/JIMM:
1. Use the traditional `juju bootstrap` command and then register the controller with JAAS.
2. Use the `jaas` CLI tool to have JIMM bootstrap a controller. Your client does not need to
access the cloud provider (AWS, Openstack, etc.) only JIMM requires access to the provider.

In order for a Juju controller to trust a JIMM controller, the `login-token-refresh-url` config option must be set to a specific URL path that serves JIMM's public key, which is used to verify signed requests when they reach the Juju controller.

This can be specified manually when bootstrapping the Juju controller directly or is set automatically when bootstrapping through JIMM.

### MicroK8s Controller

The following section provides guidance on how to connect a controller bootstrapped on MicroK8s to your JIMM running in MicroK8s.

We will name this controller `workload-microk8s` as it will be running our workloads
as opposed to our original controller which only deploys JAAS.

````{dropdown} Juju bootstrap and add

```text
juju bootstrap microk8s workload-microk8s --config login-token-refresh-url=http://jimm-endpoints.jimm.svc.cluster.local:8080/.well-known/jwks.json
```

```{note}
The hostname comes from Kubernetes DNS functionality. See more [here](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#a-aaaa-records).

Once this process is complete we will switch back to JIMM and add the controller to JIMM.

```text
juju switch jimm
juju jaas register-controller workload-microk8s --local --tls-hostname juju-apiserver
```

The `register-controller` command sends information about the controller to JIMM, which then connects to the new controller.

```{note}
A Juju server's default certificate contains a [Subject Alternative Name (SAN)](
https://en.wikipedia.org/wiki/Public_key_certificate#Subject_Alternative_Name_certificate) for the name `juju-apiserver`. This is why we specify the `--tls-hostname juju-apiserver` flag.
```

The use of the `--local` flag avoids the need to provide a public DNS address and `--tls-hostname` provides the expected
hostname used in TLS, a useful way of handling TLS issues during local development. These config options are normally not needed
in a production environment.

````

````{dropdown} JIMM bootstrap
```text
juju switch jimm
juju jaas boostrap microk8s workload-microk8s 3.6.8 --config controller-service-type=loadbalancer
```

```{note}
The desired controller version is passed to JIMM, as opposed to needing that specific version installed locally. 
```

See the `jaas` plugin's {doc}`docs <../reference/jaas-plugin>` for more details on how to bootstrap a controller on Kubernetes.

````

### LXD Controller

The following section provides guidance on how to connect a controller bootstrapped on LXD to your JIMM running in MicroK8s.

````{dropdown} Juju bootstrap and add

Run the following commands to bootstrap a LXD based controller:

```text
CLOUDINIT_FILE="cloudinit-tweak.temp.yaml"
CONTROLLER_NAME="workload-lxd"
CLOUDINIT_TEMPLATE=$'cloudinit-userdata: |
  preruncmd:
    - echo "%s    test-jimm.domain" >> /etc/hosts
  ca-certs:
    trusted:
      - |\n%s'
printf "$CLOUDINIT_TEMPLATE" "$(lxc network get lxdbr0 ipv4.address | cut -f1 -d/)" "$(cat /usr/local/share/ca-certificates/jimm-test.crt | sed -e 's/^/      /')" > "${CLOUDINIT_FILE}"
juju bootstrap lxd "${CONTROLLER_NAME}" --config "${CLOUDINIT_FILE}" --config login-token-refresh-url=https://test-jimm.domain/.well-known/jwks.json --debug
```

The set of commands will do the following:

- Create a Cloud-init template, Cloud-init provisions the LXD container that Juju will use.
- The Cloud-init script will create an entry in `/etc/hosts` to point `test-jimm.localhost` to the LXD bridge address in order to route this request to your host network.
- The Cloud-init script will add the CA cert in `/usr/local/share/ca-certificates/jimm-test.crt` to the machine. If you've placed JIMM's CA cert elsewhere, please update this file location.
- Finally the bash script will bootstrap Juju and configure it to communicate with JIMM.

Next, it is helpful to understand that we are traversing from the isolated network of the container through to
the host's network and to the LXD container where our Juju controller resides. This is possible thanks to the `host-access`
add-on in MicroK8s which allows containers to access the host network through a fixed IP address.

Connect our new controller to JIMM:
```text
juju switch jimm
juju jaas register-controller "${CONTROLLER_NAME}" --local --tls-hostname juju-apiserver
```

````


````{dropdown} JIMM bootstrap

Bootstrapping a controller to LXD via JIMM faces additional networking hurdles because JIMM needs
to communicate with the LXD server to bootstrap a controller. 

We suggest consulting the `jaas` CLI bootstrap command {doc}`reference <../reference/jaas-plugin>` docs to better understand how to
use the command in your desired use-case.

````

(control-user-access-to-a-juju-controller)=
## Control user access to a Juju controller

To grant a (collection of) user(s) access to a Juju controller, add an `audit_log_viewer` or `administrator` permission between the user(s) and the controller. For example:

```text
# Make Alice controller admin:
juju add-permission user-alice@canonical.com administrator controller-mycontroller

```

> See more: {ref}`manage-permissions`


## Remove a Juju controller

As with bootstrap there are 2 ways to remove a Juju controller attached to JIMM.

These options include:
1. Use the `jaas` plugin to first unregister the controller then use the `juju` CLI to destroy it.
2. Use the `jaas` plugin destroy the controller via an API call to JIMM, allowing for greater automation.

````{dropdown} Unregister and destroy

Switch to the JIMM controller and unregister your controller from JIMM:
```text
juju switch jimm
juju jaas unregister-controller mycontroller
```

Then switch to any non-JIMM controller and destroy your controller:
```text
juju switch mycontroller
juju destroy-controller mycontroller
```

````

````{dropdown} JIMM destroy
Switch to the JIMM controller and destroy the controller.

Note that this command will return an error if the controller is still hosting
any models. Migrate or destroy these models before destroying the controller.
```text
juju switch jimm
juju jaas destroy-controller mycontroller
```

````
