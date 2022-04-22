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
operator behaviour without full deployment. Just `run_tests` :

    ./run_tests
