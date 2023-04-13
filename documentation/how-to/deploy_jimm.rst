JAAS: Deploy JIMM
=================

Introduction
------------

In this howto we will be deploying JIMM. JIMM - Juju Intelligent Model Manager provides
the ability to manage multiple Juju models from a single point.

Prerequisites
-------------

For this tutorial you will need the following:

- A valid registered domain (regardless of the registrar)
- AWS credentials
- Basic knowledge of juju
- A subdomain registered with Route 53. To learn how to set that up, please follow :doc:`route53`.

Deploy JIMM
-----------

1. Bootstrap a controller: juju bootstrap aws
2. Download the jimm bundle from: https://drive.google.com/file/d/19IFY7m-GW1AdKUzKdKbUO_bSE6zv8tNH/view?usp=sharing 
3. Uncompress the file: ``tar xvf jimm.tar.xz``
4. Move to the jimm folder: ``cd jimm``
5. Deploy the bundle: ``juju deploy  ./bundle.yaml --overlay ./overlay-certbot.yaml``
6. Once the bundle has been deployed, get the public ip of the **haproxy/0** unit: ``juju status  --format json | jq '.applications.haproxy.units["haproxy/0"]["public-address"]'``
7. Go to the `Route 53 dashboard <https://us-east-1.console.aws.amazon.com/route53/v2/home#Dashboard>`_
8. Add an **A** record for the deployed jimm (e.g. jimm.canonical.example.com) with the IP obtained in step 6.
9. Obtain a valid certificate for the deployed candid by running: ``juju run-action --wait certbot/0 get-certificate  agree-tos=true aws-access-key-id=<Access key ID> aws-secret-access-key=<Secret access key> domains=<full dns of haproxy (e.g. jimm.canonical.example.com)> email=<Your email address>  plugin=dns-route53``
10. Let jimm know its DNS name (replace jimm.canonical.example.com with the DNS name you set up in step 8): ``juju config jimm dns-name=jimm.canonical.stimec.net``
11. Configure the URL of the Candid JIMM should use. For this tutorial we will use the official Candid: ``juju config jimm candid-url=https://api.jujucharms.com/identity``
12. Add yourself to the list of jimm administrators: ``juju config jimm controller-admins=<your USSO username>@external``

Following these steps you have deployed JIMM that uses the Canonical Candid service.

To verify that JIMM is working correctly you can try:
``juju login candid.canonical.example.com``

Once you have logged in with the Canonical Candid service you should see candid.canonical.example.com in the list of controller if you run:
``juju controllers``
