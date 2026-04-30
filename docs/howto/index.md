---
myst:
  html_meta:
    description: "How-to guides for JAAS operations including deployment management, controllers, clouds, models, users, groups, roles, and permissions."
---

(howtos)=
# How-to guides

**Step-by-step guides** covering key operations and common tasks

```{toctree}
:maxdepth: 2
:hidden:

Manage your JAAS deployment <manage-your-jaas-deployment>
Manage Juju controllers <manage-juju-controllers>
Manage clouds <manage-clouds>
Manage users <manage-users>
Manage roles <manage-roles>
Manage groups <manage-groups>
Manage permissions <manage-permissions>
Manage models <manage-models>
Manage offers <manage-offers>

```

(your-jaas-deployment-the-birds-eye-view)=
## Your JAAS deployment: the bird's eye view

Get a quick sense of how to manage your JAAS deployment, from initial deployment and configuration through observability and hardening.

- {ref}`Manage your JAAS deployment <manage-your-jaas-deployment>`

## Set up JAAS

Deploy and configure your JAAS deployment. Connect Juju controllers to JAAS. Control user access to clouds.

- {ref}`Deploy JAAS <deploy-jaas>`
- {ref}`Manage Juju controllers <manage-juju-controllers>`
- {ref}`Manage clouds <manage-clouds>`

## Handle authentication and authorization

Set up users based on roles and groups. Control access to controllers, clouds, models, and offers through permissions.

- {ref}`Manage users <manage-users>`
- {ref}`Manage roles <manage-roles>`
- {ref}`Manage groups <manage-groups>`
- {ref}`Manage permissions <manage-permissions>`

## Deploy infrastructure and applications

Create and migrate models across controllers. Control user access to models and offers. For application deployment and detailed model management, use the {external+juju:ref}`juju CLI <juju-cli>` or [Terraform Provider for Juju](https://canonical-terraform-provider-juju.readthedocs-hosted.com).

- {ref}`Manage models <manage-models>`
- {ref}`Manage offers <manage-offers>`