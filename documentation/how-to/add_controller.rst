JAAS: Add controller to JIMM
============================

Introduction
------------

JIMM gives a centralized view of all models in the system. However the work of managing 
the models is delegated to a set of standard  juju controllers deployed in various clouds
and regions.

These juju controllers must be deployed with some specific options to ensure they work
correctly in the JAAS system. This document discusses how to bootstrap a juju controller
such that it will work correctly in a JAAS system.

This document is for juju 2.x controllers, juju 3 will introduce the juju-controller
application in the controller model by default which will necessitate a (hopefully
small) reworking of this procedure. 

In this tutorial we will be bootstrapping a new juju controller in AWS and adding it to
JIMM.

Prerequisites
-------------

For this tutorial you will need the following:

- A valid registered domain (regardless of the registrar)
- AWS credentials
- Basic knowledge of juju
- A subdomain registered with Route 53. To learn how to set that up, please follow :doc:`route53`. In this tutorial we will assume that you have registered the canonical.example.com subdomain - please replace this with the appropriate subdomain that you have registered with Route 53.
- Admin access to a JIMM controller (see this tutorial). For this tutorial we will assume this JIMM is located at jimm.canonical.example.com

Deploy controller
-----------------

1. First we will prepare some parameters for the new controller and export environment variables that we will use in this tutorial. 

    The **controller name** is the name given to the controller both on the local system and within JIMM. For visibility this often includes the name of the JAAS system, the cloud, the cloud-region and some kind of unique identifier, for example jaas-aws-us-east-1-001. 

    The **cloud** is the cloud in which the controller is being bootstrapped. 

    The **cloud region** is the region in which the controller is being bootstrapped. 

    The **DNS name** is the full DNS name that will be given to the controller, often it is wise to make the hostname the same as the controller name within a particular domain. 

    The **Candid URL** is the URL of the candid server that is providing the centralized identity service for the JAAS system. 

    The **JIMM URL** is the URL of the JIMM system providing the JAAS service.

    +----------------------+----------------------+
    | Parameter            | Environment variable |
    +======================+======================+
    | Controller nam       | $NAME                |
    +----------------------+----------------------+
    | Cloud                | $CLOUD               |
    +----------------------+----------------------+
    | Cloud Region         | $REGION              |
    +----------------------+----------------------+
    | DNS name             | $DNS                 |
    +----------------------+----------------------+
    | Candid URL           | $CANDID              |
    +----------------------+----------------------+
    | JIMM URL             | $JIMM                |
    +----------------------+----------------------+


2. Now we are ready to bootstrap a controller. Please note the constraints here are the ones used for production JAAS services and should be suitable for most loads. If it is anticipated that the JAAS system will have a different model profile then we encourage you to determine the appropriate constraints for your system: 

    ``juju bootstrap --no-gui --bootstrap-constraints="root-disk=50G cores=8 mem=8G" --config identity-url=$CANDID --config allow-model-access=true --config public-dns-address=$DNS:443 $CLOUD/$REGION $NAME``

3. Next the controller should be put into HA mode: 

    ``juju enable-ha``

4. The we switch to the controller model: 

    ``juju switch controller``

5. Download the controller bundle from:

    https://drive.google.com/file/d/17GHATHXGg2GuIeIWGr0FvkguMRdv5vnH/view?usp=sharing

6. Uncompress the file: 

    ``tar xvf controller.tar.xz``

7. Move to the controller folder: 

    ``cd controller``

8. Deploy the bundle: 

    ``juju deploy  ./bundle.yaml --overlay ./overlay-certbot.yaml --map-machines=existing``

9. Once the bundle has been deployed, get the public ip of the haproxy/0 unit: 

    ``juju status  --format json | jq '.applications.haproxy.units["haproxy/0"]["public-address"]'``

10.  Go to the `Route 53 dashboard <https://us-east-1.console.aws.amazon.com/route53/v2/home#Dashboard>`_.

11.  Add an A record for the deployed controller and the DNS name specified in step 1 with the IP obtained in step 9.

12.  Obtain a valid certificate for the deployed candid by running: 

    ``juju run-action --wait certbot/0 get-certificate  agree-tos=true aws-access-key-id=<Access key ID> aws-secret-access-key=<Secret access key> domains=<dns name specified in step 1 (jimm.canonical.example.com)> email=<Your email address>  plugin=dns-route53``

13.  Install the jaas snap that you download here:

    https://drive.google.com/file/d/1LiOvVpVQ13V3x3l2PhgS2fTHDUtCEe7p/view?usp=sharing 

14. To add the bootstrapped controller to JIMM we need to create a controller-information document. To do this, run the following command:

    ``/snap/jaas/current/bin/jimmctl controller-info –public-address=$DNS:443 $NAME $NAME.yaml``

15. Now we can switch to JIMM: 
    
    ``juju switch $JIMM``

16. And add the controller to JIMM with the command: 
    
    ``/snap/jaas/current/bin/jimmctl add-controller $NAME.yaml``
    
Following these steps you added an AWS controller to your JIMM. You should now be able to add models in AWS: juju add-model test aws
