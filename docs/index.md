---
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

JAAS provides:

- The Juju Infinite Model Manager, JIMM (and its [backing charm](https://charmhub.io/juju-jimm-k8s)): A Juju enterprise-level controller.

- JIMM-specific extensions to existing Juju machinery, including

* the {doc}`jaas plugin <./reference/jaas>` which enhances the [`juju` CLI](https://documentation.ubuntu.com/juju/3.6/reference/juju-cli/),

* the [Juju dashboard](https://documentation.ubuntu.com/juju/3.6/reference/juju-dashboard/) (with its [backing charm](https://charmhub.io/juju-dashboard)), and

* the [Terraform Provider for Juju](https://documentation.ubuntu.com/terraform-provider-juju/).

When you use an existing Juju on Kubernetes controller to deploy JIMM and its dependencies, and then connect your Juju controllers to JIMM, you gain the ability to:

- use OIDC authentication for integration with your existing identity provider for federated login, service accounts, and other features offered by identity providers;
- use ReBAC for authorisation;
- use the Juju CLI, Juju Dashboard, and the Terraform Provider for Juju to interact with multiple Juju controllers from a single point of contact.

If you are a site reliability engineer looking to take Juju to the enterprise level, you need JAAS.

---------

## In this documentation
- **Learn more about JAAS:** {ref}`Architecture <jaas-architecture>`, {ref}`Security <jaas-security-overview>`
- **Set up JAAS:** {ref}`Deploy JAAS <tutorial>`, {ref}`Connect a Juju controller <add-a-juju-controller>`
- **Handle authentication and authorization:** {ref}`Set up a new user <manage-users>`, {ref}`manage-permissions`
- **Deploy infrastructure and applications:** Use {external+juju:ref}`the juju CLI <juju-cli>` or [the Terraform Provider for Juju](https://canonical-terraform-provider-juju.readthedocs-hosted.com).

````{grid} 1 1 2 2

```{grid-item-card} [Tutorial](tutorial)
:link: tutorial/index
:link-type: doc

**Start here**: a hands-on introduction to Juju for new users
```

```{grid-item-card} [How-to guides](/index)
:link: howto/index
:link-type: doc

**Step-by-step guides** covering key operations and common tasks
```

````

````{grid} 1 1 2 2
:reverse:

```{grid-item-card} [Reference](/index)
:link: reference/index
:link-type: doc

**Technical information** - specifications, APIs, architecture
```

```{grid-item-card} [Explanation](/index)
:link: explanation/index
:link-type: doc

**Discussion and clarification** of key topics
```

````

---------

## Project and community


JAAS is a member of the Ubuntu family and warmly welcomes community contributions, suggestions, fixes and constructive feedback.

* [Release notes](https://github.com/canonical/jimm/releases)
* [Code of conduct](https://ubuntu.com/community/ethos/code-of-conduct)
* [Join our chat](https://matrix.to/#/#jimm:ubuntu.com)
* [Join our forum](https://discourse.charmhub.io/)
* [Report a bug](https://github.com/canonical/jimm/issues)
* [Contribute](https://github.com/canonical/jimm/blob/v3/CONTRIBUTING.md)
* [Visit our careers page](https://canonical.com/careers/engineering)

Thinking about using Juju for your next project? [Get in touch!](https://canonical.com/contact-us)

