Controller Bundle
=================

This bundle deploys a highly-available controller system, suitable for use in JAAS.

Prerequisits
------------

In order to deploy the bundle the following configuration items need to
be prepared:

### TLS Certificates

Get appropriate certificates from your CA and store the certificate
chain in `LOCAL/controller.crt`, and the private key in `LOCAL/controller.key`.

Deployment
----------

This bundle needs to be deployed on top of an already existing controller
model.

To bootstrap an appropriate model run commands like the following:
    juju bootstrap --bootstrap-constraints="cores=8 mem=8G root-disk=50G" --config identity-url=<candid URL> --config allow-model-access=true --config public-dns-address=<DNS of the controller>:443 <cloud>/<region> <name> 
    juju enable-ha -n 3
    juju switch controller

To deploy the bundle into the model run:

    juju deploy --map-machines=existing ./bundle.yaml --overlay overlay-certificate.yaml
