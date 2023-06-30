# JAAS Intelligent Model Manager

## Description

JIMM provides centralized model management for JAAS systems.

## Usage

The JIMM payload is provided by a JIMM snap that must be attached to
the application:

```
juju deploy ./jimm-k8s.charm --resource jimm-image=jimm:latest
```

To upgrade the workload attach a new version of the jimm container:

```
juju attach jimm-k8s jimm-image=jimm:latest
```

JIMM requires a postgresql database for data storage:

```
juju jimm-k8s dsn='postgres://...'
```

## Developing

Create and activate a virtualenv with the development requirements:

    virtualenv -p python3 venv
    source venv/bin/activate
    pip install -r requirements-dev.txt

## Testing

The Python operator framework includes a very nice harness for testing
operator behaviour without full deployment. The test suite can be run 
using `tox`. You can either `pip install tox` system-wide or create a
virtual env and install tox there as follows
```
python3 -m venv venv
source ./venv/bin/activate
pip install tox
```
At this point you can run tests/linters/formatters.
```
tox -e fmt
tox -e lint
tox -e unit
tox -e integration
```
Note that integration tests will build the charm and deploy it to a local
microk8s controller (which must be setup prior to running the integration test).
To switch the integration test to use a locally built charm use 
```
tox -e integration -- --localCharm
```
