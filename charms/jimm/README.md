# JAAS Intelligent Model Manager

## Description

JIMM provides centralized model management for JAAS systems.

## Usage

The JIMM payload is provided by a JIMM snap that must be attached to
the application:

```
juju deploy ./jimm.charm --resource jimm-snap=jimm.snap
```

To upgrade the workload attach a new version of the snap:

```
juju attach juju-jimm jimm-snap=jimm.snap
```
## Dependencies

### Postgresql

JIMM requires a postgresql database for data storage:

```
juju deploy postgresql
juju add-relation juju-jimm:database postgresql:database
```


## Developing

Create and activate a virtualenv with the development requirements:

    virtualenv -p python3 venv
    source venv/bin/activate
    pip install -r requirements-dev.txt

## Testing

The Python operator framework includes a very nice harness for testing
operator behaviour without full deployment. Just `run_tests`:

    ./run_tests
