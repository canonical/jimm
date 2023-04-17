JAAS: Deploy Candid on K8S
==========================

Introduction
------------

In this howto we will be deploying Candid on Kubernetes.  Candid provides a macaroon-based authentication service that is able to utilize many standard identity providers:

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
- A subdomain registered with Route 53. To learn how to set that up, please follow :doc:`route53`.
- Access to a Kubernetes cluster
- Access to a Postgresql database

Deploy a Kubernetes cluster
---------------------------

In case you do not already have access to a Kubernetes cluster, you can deploy one on AWS. Start by bootstrapping a new controller in AWS:

``juju bootstrap aws k8s-controller``

and then deploy the kubernetes-core bundle:

``juju deploy kubernetes-core``

and the aws-integrator charm:

``juju deploy aws-integrator``

which simplifies working with charmed Kubernetes on AWS.
Use the following commands to configure and relate aws-integrator to various applications:

.. code:: console

    juju trust aws-integrator
    juju relate aws-integrator kubernetes-control-plane
    juju relate aws-integrator kubernetes-worker

Once all applications settle down and start fetch the config file that will let you use kubectl:

``juju scp kubernetes-control-plane/0:config ~/.kube/config``

Deploy Candid
-------------

Once you have access to a K8s cluster, you can verify it by running:

``kubectl get nodes``

and if that works add the kubernetes cluster as a cloud to your juju client:

``juju add-k8s myk8s``

We can the bootstrap a new controller in the added k8s cluster by running:

``juju bootstrap myk8s infrastructure``

and when this process finishes we can add a new model for Candid:

``juju add-model candid``

Now we can deploy the Candid into the newly created model:

``juju deploy candid-k8s –channel edge``

As Juju does not currently support exposing an application on a k8s cloud, we need to also deploy nginx-ingress-integrator charm. Run:

.. code:: console
    
    juju deploy nginx-ingress-integrator ingress
    juju trust ingress --scope=cluster
    juju relate ingress candid-k8s

Once deployed go to the management console of your domain and create an A record for the deployed Candid (e.g. candidcanonical.example.com) with the IP of k8s worker nodes. 

Configure Candid
----------------

Once deployed we need to configure Candid. Run the following commands:

.. code:: console

    juju config candid-k8s location=https://candid.canonical.stimec.net
    juju config candid-k8s admin-agent-public-key='test-public-key'

    juju config candid-k8s identity-providers='- type: static                                 
    name: static
    domain: stimec
    description: Default identity provider
    users:
        admin:
        name: User One
        email: admin
        password: password1
        groups:
        - group1
        user1:
        name: User Two
        email: user1
        password: password1
        groups:
        - group2
        my-user:
        name: User Three
        email: my-user
        password: password1
        groups:
        - group2'
    juju config candid-k8s postgresql-dsn=<postgresql dsn>


Next we also need to configure ingress. Usually the Kubernetes cluster operator will set up a Kubernetes secret for you containing a certificate and key for Candid’s FQDN (in case you need a certificate look at the next section) and give you the secret name. All you need to do next is:

``juju config ingress tls-secret-name=<secret name>``

and the ingress charm will get certificates from the Kubernetes secret and set up TLS for you.
Now you test Candid by opening your browser and navigating to https://candid.<your domain>/login

Appendix
--------

Don’t have a postgresql database
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

In case you do not have access to a Postgresql database you can use Amazon’s RDS to create one. Navigate to the RDS console and select “Create database”. Under “Engine type” select “PostgreSQL”, specify “Master username” and “Master password”. Also make sure to select “Public access” as “Yes”. You can customize all other options to your preference. Once the database is created, navigate to the database’s dashboard. There you will see the “Endpoint” adn “Port” strings, which you will need to connect to the database.  Use the following command to configure Candid:

``juju config candid-k8s dns=’postgres://<master username>:<master password>@<database endpoint>:<database port>/<database name>``

Don’t have certificates
~~~~~~~~~~~~~~~~~~~~~~~

In case the cluster operator did not set up a Kubernetes secret for you containing certificate and key for Candid’s FQDN, you can use Let’s Encrypt and cert-manager to get the certificate.
First, you will need to install helm:

``sudo snap install helm``

Then run the following commands to deploy cert-manager:

.. code:: console

    helm repo add jetstack https://charts.jetstack.io
    helm repo update
    helm install certmgr jetstack/cert-manager
        --set installCRDs=true \
        --version v1.9.1 \
        --namespace cert-manager \
        --create-namespace

Since the production Let’s Encrypt servers do some fancy rate limiting 
and we don’t want to exceed the limit, we will first test our setup with 
the staging server.

Create a fille stg-issuer.yaml with the following content:

.. code:: yaml
    
    apiVersion: cert-manager.io/v1
    kind: Issuer
    metadata:
    name: letsencrypt-staging
    spec:
    acme:
        # The ACME server URL
        server: https://acme-staging-v02.api.letsencrypt.org/directory
        # Email address used for ACME registration
        email: <your email>
        # Name of a secret used to store the ACME account private key
        privateKeySecretRef:
        name: letsencrypt-staging
        # Enable the HTTP-01 challenge provider
        solvers:
        - http01:
            ingress:
            class:  nginx

and run:

``kubectl apply -n candid -f stg-issuer.yaml``

which will create a certificate issuer in candid’s namespace.

The create stg-certs.yaml file with the following content:

.. code:: yaml 

    apiVersion: cert-manager.io/v1
    kind: Certificate
    metadata:
    name: candid-stg-cert  #name of this object
    namespace: candid #same namespace as 
    spec:
    dnsNames:
        - candid.canonical.stimec.net
    secretName: letsencrypt-stg-certs
    issuerRef:
        name: letsencrypt-staging
        kind: Issuer

and run:

``kubectl apply -n candid -f stg-certs.yaml``

This should create a certificate and key using the staging issuer. Inspect the created certificate by running:

``kubectl describe certificate -n candid candid-stg-cert``

and:

``kubectl describe secret letsencrypt-stg-certs -n candid``

which will show a Kubernetes secret and in its data you should see a stored tls.crt and tls.key.
If this all worked (and i have no doubt it did :) ), then we can proceed by creating a production issuer. Create a prod-issuer.yaml file with the following content:

.. code:: yaml

    apiVersion: cert-manager.io/v1
    kind: Issuer
    metadata:
    name: letsencrypt-prod
    spec:
    acme:
        # The ACME server URL
        server: https://acme-v02.api.letsencrypt.org/directory
        # Email address used for ACME registration
        email: ales.stimec@canonical.com
        # Name of a secret used to store the ACME account private key
        privateKeySecretRef:
        name: letsencrypt-prod
        # Enable the HTTP-01 challenge provider
        solvers:
        - http01:
            ingress:
            class: nginx

and run:

``kubectl apply -n candid -f prod-issuer.yaml``

Then create a prod-certs.yaml file with the following content:

.. code:: yaml

    apiVersion: cert-manager.io/v1
    kind: Certificate
    metadata:
    name: candid-cert  #name of this object
    namespace: candid
    spec:
    dnsNames:
        - candid.canonical.stimec.net
    secretName: letsencrypt-certs
    issuerRef:
        name: letsencrypt-prod
        kind: Issuer

and run:

``kubectl apply -n candid -f prod-certs.yaml``

This will create a letsencrypt-certs secrets for you, which you can inspect by running:

``kubectl describe secret letsencrypt-certs -n candid``

which will show the created secret and in its data you should see a stored tls.crt and tls.key.
To see the certificate data run:

``kubectl describe certificate -n candid candid-cert``


Once you have the production certificate, you can configure the ingress application by running:

``juju config ingress tls-secret-name=letsencrypt-certs``


