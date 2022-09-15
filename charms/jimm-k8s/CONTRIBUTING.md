# jimm

## Developing

Create and activate a virtualenv with the development requirements:

    virtualenv -p python3 venv
    source venv/bin/activate
    pip install -r requirements-dev.txt

## Intended use case

This JIMM operator charm is intended for deploying the 
Juju Intelligent Model Manager in a k8s cluster. The charm
does not use relations to connect to postgresql and/or
vault as it is assumed those services could be deployed
in a different model.

## Roadmap

* Add postgresql relation
* Add vault relation (when a k8s vault charm becomes available)

## Testing

The Python operator framework includes a very nice harness for testing
operator behaviour without full deployment. Just `run_tests` :

    ./run_tests
