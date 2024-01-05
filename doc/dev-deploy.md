# Deploy JAAS

By following this document you will be able to deploy a complete JAAS
system: Candid with one or more identity providers, JIMM and controllers
added to JIMM.

## Prerequisites

### Domain

To deploy JAAS we need a domain as each service needs a separate DNS entry for each of the deployed service. This will enable us to use
certbot to obtain valid certificates from Let's Encrypt.

We can use AWS Route 53 to host a subdomain as certbot has a
dns-route53 plugin. To achieve that we need to obtain an 
**aws-secret-access-key** and **aws-access-key-id** from Route 53.
In my case i set up AWS Route 53 to manage the canonical.stimec.net 
subdomain and i will use this as an example domain for the rest of this
document.

### Deployment bundles

Deployment bundles for JAAS can be found in 

    git.launchpad.net/canonical-jaas

You can check it out by running:

    git clone git+ssh://git.launchpad.net/canonical-jaas

## OpenLDAP

### Deploy

We can either bootstrap a new controller or create a new model for the 
LDAP deploy:

    juju bootstrap aws dev-ldap

Then we deploy an ubuntu unit and certbot:

    juju deploy ubuntu ldap
    juju deploy cs:~yellow/certbot

### Install OpenLDAP

SSH into the unit:

    juju ssh ldap/0

Install LDAP by running:

    sudo apt install slapd ldap-utils

Having installed OpenLDAP run:

    sudo dpkg-reconfigure slapd

Here i configured LDAP so that admin is:

    cn=admin,dc=ldap,dc=canonical,dc=stimec,dc=net

And specified a password for the admin user. In the remainder of this 
document you will see *dc=ldap, dc=canonical, dc=stimec, dc=net* in
command examples - make sure to replace this with the actual domain you
set up in ldap.

### Configure TLS

Then we can exit from the LDAP unit and configure certbot by specifying certificate, key and trust chain paths:

    juju config certbot chain-path=/etc/ldap/ldap-chain.pem
    juju config certbot key-path=/etc/ldap/ldap-key.pem
    juju config certbot cert-path=/etc/ldap/ldap-cert.pem

And add a relation between ldap and certbot:

    juju add-relation certbot ldap

Next we then need to create a DNS **A** record for the LDAP unit. Let's
say we create ldap.<**domain**> that will point to the IP of the 
LDAP unit.

Then, to obtain a certficate we run:

    juju run-action --wait certbot/0 get-certificate  agree-tos=true aws-access-key-id=<**aws-secret-access-key-id**> aws-secret-access-key=<**aws-secret-access-key**>
    domains=ldap.<**domain**> email=<**developer email**>  plugin=dns-route53

This will result in creating of .pem files specified by the certbot config
above and we can proceede setting up out LDAP instance.

    juju ssh ldap/0

    cd /etc/ldap

Create file certinfo.ldif with the following content

    dn: cn=config
    replace: olcTLSCACertificateFile
    olcTLSCACertificateFile: /etc/ldap/ldap-chain.pem
    -
    replace: olcTLSCertificateFile
    olcTLSCertificateFile: /etc/ldap/ldap-cert.pem
    -
    replace: olcTLSCertificateKeyFile
    olcTLSCertificateKeyFile: /etc/ldap/ldap-key.pem

And then change the ldap configuration

    sudo ldapmodify -Y EXTERNAL -H ldapi:/// -f certinfo.ldif

This will make LDAP use certificates provided by the certbot.

### LDAP Users

Then to set up LDAP we create content.ldif with the following content:

    dn: ou=People,dc=ldap,dc=canonical,dc=stimec,dc=net
    objectClass: organizationalUnit
    ou: People

    dn: ou=Groups,dc=ldap,dc=canonical,dc=stimec,dc=net
    objectClass: organizationalUnit
    ou: Groups

    dn: cn=miners,ou=Groups,dc=ldap,dc=canonical,dc=stimec,dc=net
    objectClass: posixGroup
    cn: miners
    gidNumber: 5000

To create an LDAP user we then create a file named <**username**>.ldif with
the following content:

    dn: uid=<**username**>,ou=People,dc=ldap,dc=canonical,dc=stimec,dc=net
    objectClass: inetOrgPerson
    objectClass: posixAccount
    objectClass: shadowAccount
    uid: <**username**>
    sn: 2
    givenName: <**name**>
    cn: <**username**>
    displayName: <**display name**>
    uidNumber: <**uuid number e.g. 10000**>
    gidNumber: <**gid number e.g. 5000**>
    userPassword: {CRYPT}x
    gecos: <**display name**>
    loginShell: /bin/bash
    homeDirectory: /home/<**username**>

And then run:

    ldapadd -x -D cn=admin,dc=ldap,dc=canonical,dc=stimec,dc=net -W -f <**username**>.ldif

To set a password for the created user run:

    ldappasswd -x -D cn=admin,dc=ldap,dc=canonical,dc=stimec,dc=net -W -S uid=<**username**>,ou=People,dc=ldap,dc=canonical,dc=stimec,dc=net

## Candid

We can either bootstrap a new controller or create a new model for the 
Candid deploy:

    juju bootstrap aws dev-candid

Now go into the folder where you checked out canonical-jaas and into
the /bundles/candid folder.

To deploy candid run (since we are using certbot to obtain certificates):

    juju deploy ./bundle.yaml --overlay ./overlay-certbot.yaml

This will deploy 2 candid units, 1 postgresql unit and 2 haproxy units. Feel free to modify the bundle to reduce cpu, memory or root disk contraints.

Once deployed, we need to create a DNS **A** record for the two haproxy
units - in my case candid.canonical.stimec.net pointed to the one haproxy
unit as haproxy seems to have a problem with peer-to-peer relations that
i couldn't be bothered to resolve.

Now, to obtain a certificate for haproxy we run:

    
    juju run-action --wait certbot/0 get-certificate  agree-tos=true aws-access-key-id=<**aws-secret-access-key-id**> aws-secret-access-key=<**aws-secret-access-key**>
    domains=candid.<**domain**> email=<**developer email**>  plugin=dns-route53

Then we need to configure Candid.

Set the location:

    juju config candid location=https://candid.canonical.stimec.net

To set up identity providers follow instruction on setting up ldap or
azure then use:

    juju config candid identity-providers= `<**list of identity providers**>`

### LDAP Identity Provider

Then we need to set identity-providers configuration option to 

    - type: ldap
      name: TestLDAP
      description: LDAPLogin
      domain: ldap.canonical.stimec.net
      url: ldap://ldap.canonical.stimec.net/dc=ldap,dc=canonical,dc=stimec,dc=net
      dn: cn=admin,dc=ldap,dc=canonical,dc=stimec,dc=net
      password: <LDAP admin password>
      user-query-filter: (objectClass=inetOrgPerson)
      user-query-attrs:
          id: uid
          email: mail
          display-name: displayName
      group-query-filter: (&(objectClass=groupOfNames)(member={{.User}}))
      hidden: false
      ca-cert: |
          <<LDAP's CA certificate, which is the contents of the /etc/ldap/ldap-chain.pem in the ldap unit>>

### Azure Identity Provider

If, for some reason you want to add Azure an identity provider go to the
[Azure portal](https://portal.azure.com/) and find **App Registration**. 
Fill in the name (e.g. Development Candid). 
For **Supported account types** select **Accounts in any organizational directory (Any Azure AD directory - Multitenant) and personal Microsoft accounts (e.g. Skype, Xbox)**. 
For **Redirect URI** select **Web** and enter

    https://candid.azure.canonical.com/login/azure/callback

as the redirect URI.

Then go to the created **App Registration** and copy **Application (client) ID** (this is **client_id**). The go to **Certificates & secrets**. Create a **New client secret** and copy it's value (this is
**client_secret**). 

Now we need to make Candid aware of the new identity provider. To do that we need to add another item to the identity_providers configuration value. We need to add the following item:

    - type: azure
      client-id: <**client_id**>
      client-secret: <**client_secret**>

## JIMM

We can either bootstrap a new controller or create a new model for the 
JIMM deploy:

    juju bootstrap aws dev-jimm

Now go into the folder where you checked out canonical-jaas and into
the /bundles/jimm folder.

To deploy jimm run (since we are using certbot to obtain certificates):

    juju deploy ./bundle.yaml --overlay ./overlay-certbot.yaml

This will deploy 2 jimm units, 1 mongodb unit and 2 haproxy units. Feel free to modify the bundle to reduce cpu, memory or root disk contraints.

Once deployed, we need to create a DNS **A** record for the two haproxy
units - in my case jimm.canonical.stimec.net pointed to the one haproxy
unit as haproxy seems to have a problem with peer-to-peer relations that
i couldn't be bothered to resolve.

Now, to obtain a certificate for haproxy we run:

    
    juju run-action --wait certbot/0 get-certificate  agree-tos=true aws-access-key-id=<**aws-secret-access-key-id**> aws-secret-access-key=<**aws-secret-access-key**>
    domains=jimm.<**domain**> email=<**developer email**>  plugin=dns-route53

Then we need to configure JIMM to use the deployed Candid:

    juju config identity-location=https://candid.<**domain**>

And add a controller admin, which should be one of the created LDAP users:

    juju config jimm controller-admins=<**username**>@ldap.<**domain**>

## JIMM Controllers

To bootstrap and add controllers to JIMM please follow the guide that can
be found [here](https://docs.google.com/document/d/1rtJne7CV6dRsKvCUE85BA5adPvEa5V-TczKSAMzxP9M/edit#heading=h.yij8xaij9lcy).
