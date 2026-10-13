---
myst:
  html_meta:
    description: "Complete reference for JAAS including technical documentation, APIs, security, JAAS plugin commands, and entity specifications."
---

(reference)=
# Reference

Technical specifications, APIs, and comprehensive details of all JAAS components.

## Platform

JAAS is a multi-controller orchestration platform that coordinates Juju controllers and enhances resource management across cloud deployments. Which Juju versions it works with is covered by the supported Juju versions reference.

- {ref}`jaas-supported-juju-versions`

### Plugin

You interact with JAAS through the JAAS plugin -- a CLI extension that provides commands for managing multi-controller deployments and access control.

- {doc}`jaas plugin <./jaas-plugin>`

### JIMM controller

JIMM (Juju Intelligent Model Manager) is the server component that powers JAAS, providing the central controller that coordinates multiple Juju controllers. It is deployed through the [`jimm` charm](https://charmhub.io/juju-jimm-k8s) and provides audit logging for tracking operations and changes across all managed controllers.

- {ref}`audit-logs`

## Users

JAAS extends Juju's authentication with groups and roles, providing fine-grained access control for users across multiple controllers.

- {ref}`user`
- {ref}`group`
- {ref}`role`

## Infrastructure and applications

JAAS manages Juju entities -- clouds, controllers, models, and offers -- with multi-controller coordination and centralized access control.

- {ref}`cloud`
- {ref}`controller`
- {ref}`model`
- {ref}`offer`

```{toctree}
:titlesonly:
:glob:
:hidden:

*
```
