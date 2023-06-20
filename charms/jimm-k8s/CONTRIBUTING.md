# Contributing

## Overview

This documents explains the processes and practices recommended for contributing enhancements to
this operator.

- If you would like to chat with us about your use-cases, you can reach
  us at [Discourse](https://chat.charmhub.io/charmhub/channels/jaas).
- Familiarising yourself with the [Charmed Operator Framework](https://juju.is/docs/sdk) library
  will help you a lot when working on new features or bug fixes.
- All enhancements require review before being merged. Code review typically examines
  - code quality
  - test coverage
  - user experience for Juju administrators this charm.
- Please help us out in ensuring easy to review branches by rebasing your pull request branch onto
  the `main` branch. This also avoids merge commits and creates a linear Git commit history.

## Developing

You can create an environment for development with `tox`:

```shell
virtualenv -p python3 venv
source venv/bin/activate
pip install -r requirements-dev.txt
pip install tox
```

The charm additionally requires the following relations:
- ingress, interface: ingress
- database, interface: postgresql_client
- vault, interface: vault-kv
- openfga, interface: openfga
- certificates, interface: tls-certificates

### Testing

```shell
tox -e fmt           # update your code according to linting rules
tox -e lint          # code style
tox -e unit          # unit tests
tox -e integration   # integration tests
tox                  # runs 'lint' and 'unit' environments
```


## Build charm

Build the charm in this git repository using:

```shell
charmcraft pack
```

### Deploy

```bash
# Create a model
juju add-model dev
# Enable DEBUG logging
juju model-config logging-config="<root>=INFO;unit=DEBUG"
# Deploy the charm
juju deploy ./juju-jimm-k8s_ubuntu-22.04-amd64.charm
```
