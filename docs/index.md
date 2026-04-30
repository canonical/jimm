---
myst:
  html_meta:
    description: "JAAS documentation covering the Juju Intelligent Model Manager (JIMM) with enterprise authentication, ReBAC authorization, and centralized Juju control."
relatedlinks: "[Charmcraft](https://documentation.ubuntu.com/charmcraft/), [Charmlibs](https://canonical-charmlibs.readthedocs-hosted.com/), [Concierge](https://github.com/canonical/concierge), [Jubilant](https://documentation.ubuntu.com/jubilant/), [Juju](https://documentation.ubuntu.com/juju/), [Ops](https://documentation.ubuntu.com/ops/), [Pebble](https://documentation.ubuntu.com/pebble/), [Terraform &nbsp; Provider &nbsp; for &nbsp; Juju](https://documentation.ubuntu.com/terraform-provider-juju/)"
---

(home)=
# JAAS documentation

```{toctree}
:maxdepth: 2
:hidden: true

tutorial/index
howto/index
reference/index
explanation/index
```

JAAS is an enterprise layer on top of [Juju](https://documentation.ubuntu.com/juju/).

JAAS provides JIMM (the Juju Infinite Model Manager), a Juju enterprise-level controller, as well as JIMM-specific extensions to the [`juju` CLI](https://documentation.ubuntu.com/juju/3.6/reference/juju-cli/), the [Juju dashboard](https://documentation.ubuntu.com/juju/3.6/reference/juju-dashboard/), and the [Terraform Provider for Juju](https://documentation.ubuntu.com/terraform-provider-juju/).

When you use an existing Juju controller to deploy JIMM and its dependencies, and then connect your Juju controllers to JIMM, you gain the ability to use [OIDC](https://openid.net/developers/how-connect-works/) to authenticate with your Juju controller, use [ReBAC](https://auth0.com/blog/relationship-based-access-control-rebac/) for authorization, and interact with multiple Juju controllers from a single point of contact.

If you are a site reliability engineer looking to take Juju to the enterprise level, you need JAAS.

## In this documentation
- **Learn more about JAAS:** {ref}`Get started with JAAS <tutorial>` • {ref}`Architecture <jaas-architecture>` • {ref}`Security <jaas-security-overview>`
- **Set up JAAS:** {ref}`Deploy JIMM <tutorial>` • {ref}`Add a Juju controller <add-a-juju-controller>`
- **Handle authentication and authorization:** {ref}`Add a user <add-a-user>` • {ref}`Manage user access <control-user-access>`
- **Deploy infrastructure and applications:** Use {external+juju:ref}`the juju CLI <juju-cli>` or [the Terraform Provider for Juju](https://canonical-terraform-provider-juju.readthedocs-hosted.com).

## How this documentation is organised

This documentation uses the [Diátaxis documentation structure](https://diataxis.fr/).

- The {ref}`Tutorial <tutorial>` takes you step-by-step through setting up JAAS, connecting controllers, and managing permissions.
- {ref}`How-to guides <howtos>` assume you have basic familiarity with JAAS and Juju.
- {ref}`Reference <reference>` provides technical specifications and command references.
- {ref}`Explanation <explanation>` includes architecture overviews, security models, and detailed discussions.

## Project and community

JAAS is a member of the Ubuntu family. It's an open source project that warmly welcomes community contributions, suggestions, fixes and constructive feedback.

### Get involved

* [Join our chat](https://matrix.to/#/#jimm:ubuntu.com)
* [Join our forum](https://discourse.charmhub.io/)
* [Report a bug](https://github.com/canonical/jimm/issues)
* [Contribute](https://github.com/canonical/jimm/blob/v3/CONTRIBUTING.md)

### Releases

* [Release notes](https://github.com/canonical/jimm/releases)

### Governance and policies

* [Code of conduct](https://ubuntu.com/community/ethos/code-of-conduct)
* [Visit our careers page](https://canonical.com/careers/engineering)

Thinking about using Juju for your next project? [Get in touch!](https://canonical.com/contact-us)

