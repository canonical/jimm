JAAS: Setting up Route53
========================

Introduction
------------


In this document will we will show you how to use Route53 to host a subdomain of the
domain you already own and how to use the certbot charm to obtain a valid certificate.

Prerequisites
-------------

For this tutorial you will need the following:

- A valid registered domain (regardless of the registrar)

- AWS credentials

- Basic knowledge of juju

Creating a hosted zone for the subdomain
----------------------------------------

In this tutorial we will assume that you have registered the example.com domain and by
following the steps below you will register a new hosted zone with Route 53 for a 
canonical.example.com subdomain.

1. Go to the Route 53 dashboard.
2. In the navigation pane, choose **Hosted zones**.
3. Choose **Create hosted zone**.
4. Enter the name of the subdomain (example: canonical.example.com)
5. For **Type**, choose **Public hosted zone**.
6. Choose **Created hosted zone**.
7. Route 53 will automatically assign four name servers to the newly created zone. To start using the hosted zone for the subdomain you must create a new name server (NS) record in for the domain (example.com): the name of the NS record must be the same as the name of the subdomain (from the example above: canonical.example.com) and for the value set the names of the name servers assigned to your hoste zone by Route 53.

Now you have registered the canonical.example.com subdomain as a public hosted zone with Route 53. Record the Hosted zone ID, which you will need in the next step.

Obtain credentials
------------------

In order to be able to use certbot or any other tool that uses programmatic access to obtain valid certificates for your hosts, you will need to create a user and give it rights to use the created hosted zone. 

1. Go to `IAM Console <https://console.aws.amazon.com/iam/>`_.
2. Choose **Users** and then choose **Add users**.
3. Type the name for the new user.
4. Select **Access key - Programmatic access** under the **Select AWS credential type**.
5. In the next step choose **Attach existing policies directly**.
6. Choose **Create policy**.
7. Choose **JSON**.
8. Paste the following:
   
.. code:: json

    {
        "Version": "2012-10-17",
        "Id": "certbot-dns-route53 sample policy",
        "Statement": [
            {
                "Effect": "Allow",
                "Action": [
                    "route53:ListHostedZones",
                    "route53:GetChange"
                ],
                "Resource": [
                    "*"
                ]
            },
            {
                "Effect": "Allow",
                "Action": [
                    "route53:ChangeResourceRecordSets"
                ],
                "Resource": [
                    "arn:aws:route53:::hostedzone/<Your hosted zone ID>"
                ]
            }
        ]
    }

9. Replace the field **<Your hosted zone ID>** with the hosted zone ID you noted down previously. 
10. Type in the **Name** (e.g. route53_access_policy) for the policy and choose **Create policy**.
11. When the policy is created, return to the user creation process. Choose the created policy for the user, review information and choose **Create user**.
12. Copy the **Access key ID** and **Secret access key** credentials.
    
Using Certbot to obtain certificates
------------------------------------

1. Bootstrap a juju controller (in aws, azure or gce)
2. Deploy haproxy
3. Deploy certbot
4. Add relation between haproxy and certbot by running: ``juju relate haproxy certbot``
5. Set the **combined-path** configuration option to the default path for haproxy by running: ``juju config certbot combined-path=/var/lib/haproxy/default.pem``
6. Wait for the deploy
7. Check which unit of haproxy and certbot has been deployed. In the following steps we assume that these are **haproxy/0** and **certbot/0** units. Please replace those with appropriate values for your deployment in the following steps.
8. Run the following command to get the public IP of the haproxy unit: ``juju status  --format json | jq '.applications.haproxy.units["haproxy/0"]["public-address"]``
9. Go to the Route 53 dashboard
10. Choose **Hosted zone** and then the zone you created.
11. Choose **Create Record**
12. In the **Record name** enter the desired dns name (e.g. demo) and in the value paste the public IP address of the haproxy unit, then choose **Create records**.
13. Run action on the certbot unit to obtain the certificate: ``juju run-action --wait certbot/0 get-certificate  agree-tos=true aws-access-key-id=<Access key ID> aws-secret-access-key=<Secret access key> domains=<full dns of haproxy (e.g. demo.canonical.example.com)> email=<Your email address>  plugin=dns-route53``

The result of this should be a deployed haproxy with a valid certificate. In case of 
errors, please re-check the configuration of your domain’s NS entries at the registrar’s
page and on Route 53.

