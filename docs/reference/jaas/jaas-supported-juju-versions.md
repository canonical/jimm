(jaas-supported-juju-versions)=
# Supported Juju versions

The following sections describe which version of the Juju CLI or controller is required for different scenarios.

## Deploying JAAS

In order to deploy JAAS and all its components you must use a Juju controller with a minimum version of **3.x**.

Juju 3.x is required for the support of Juju secrets.

## Using JAAS

In order to interact with JAAS as a user, you must use a Juju CLI with a minimum version of **3.6.4**.

Previous versions of the Juju CLI do not include the necessary functionality to authenticate with JAAS.

## Add controllers to JAAS

JAAS supports communicating with Juju controllers of various versions.

JAAS supports Juju controllers with a minimum version **3.4**.

Controllers with a lower version do not have the necessary configuration options to work with JAAS.

Going forward, JAAS is versioned with the same major version number as Juju. This means JAAS will always
support Juju controllers with the same major version as itself. E.g. JAAS v3 should be used with Juju 3.x controllers.

Additionally, JAAS will also support the last LTS release from Juju's previous major release E.g. JAAS v4 will also
support the final Juju 3.x minor version.

More information on Juju's roadmap and release information can be found [here](https://juju.is/docs/juju/roadmap).

## Using the Juju Terraform Provider

The Juju Terraform Provider can be used with JAAS to provision your models.
Additionally, JAAS specific resources can be used to configure access rules.
Read the docs for more information on the [Juju Terraform Provider](https://registry.terraform.io/providers/juju/juju/latest/docs).

The minimum version of JAAS that supports all functionality of the Terraform provider is v3.1.11.
The minimum recommended version of the Terraform provider to use with JAAS is v0.15.0.
