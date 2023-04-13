JAAS: Deploy Candid
===================

Introduction
In this tutorial we will be deploying Candid. Candid provides a macaroon-based authentication service that is able to utilize many standard identity providers:

- UbuntuSSO
- LDAP
- Google OpenID Connect
- ADFS OpenID Connect
- Azure OpenID Connect
- Keystore (userpass or token)
- Static identity provider (only used for testing)

Prerequisites
-------------

For this tutorial you will need the following:

- A valid registered domain (regardless of the registrar)
- AWS credentials
- Basic knowledge of juju
- A subdomain registered with Route 53. To learn how to set that up, please follow :doc:`route53`. In this tutorial we will assume that you have registered the canonical.example.com subdomain - please replace this with the appropriate subdomain that you have registered with Route 53.

Deploy Candid
-------------

1. Bootstrap a controller: 
   ``juju bootstrap aws``
2. Download the candid bundle from: 
   https://drive.google.com/file/d/1ZyZeI0jNacbXK-AgxzUT0IUEp9tQ85QH/view?usp=sharing

3. Uncompress the file: 
   ``tar xvf candid_v1.11.0.tar.xz ``

4. Move to the candid folder: 
   ``cd candid``

5. Deploy the bundle: 
   ``juju deploy ./bundle.yaml --overlay ./overlay-certbot.yaml``

6. Once the bundle has been deployed, get the public ip of the haproxy/0 unit: 
    ``juju status --format json | jq '.applications.haproxy.units["haproxy/0"]["public-address"]'``

7. Go to the `Route 53 dashboard <https://us-east-1.console.aws.amazon.com/route53/v2/home#Dashboard>`_.

8. Add an A record for the deployed candid (e.g. candid.canonical.example.com) with the IP obtained in step 6.

9. Obtain a valid certificate for the deployed candid by running: 
    ``juju run-action --wait certbot/0 get-certificate  agree-tos=true aws-access-key-id=<Access key ID> aws-secret-access-key=<Secret access key> domains=<full dns of haproxy (e.g. candid.canonical.example.com)> email=<Your email address>  plugin=dns-route53``

10. Let candid know its DNS name (replace candid.canonical.example.com with the DNS name you set up in step 8): 
    ``juju config candid location=https://candid.canonical.example.com``

11. Now all that is left is to set up the identity providers. In this tutorial we will set up a static identity provider with hard-coded usernames and passwords: 

    .. code::

        juju config candid identity-providers='- type: static
        name: static
        description: Default identity provider
        users:
            user1:
            name: User One
            email: user1
            password: s3cre7Pa55w0rd1
            groups:
            - group1
            - group3
        user2:
            name: User Two
            email: user2
            password: s3cre7Pa55w0rd2
            groups:
            - group2
            - group3'

Following these steps you have deployed Candid that uses a static identity provider 
with two hardcoded users. **Please note that the static identity provider should not
be used in production**.

To verify that Candid is working correctly, open your browser and go to 
https://<candid DNS>/login and try to log in as “user1” or “user2” using one of the 
hardcoded passwords.