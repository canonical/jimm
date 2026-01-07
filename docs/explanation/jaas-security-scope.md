(jaas-security-scope)=
# Security scope

<!--   TODO (Kian):
   Much of this document's content is now covered in the security doc in reference/security and reused where possible.
   We should consider what parts of this doc to remove and where to better put the parts that we keep.
-->

The scope of JAAS' security covers multiple aspects, including:

## Secure Communication

JAAS ensures secure communication between controllers (management nodes) and models (namespaces)
by means of authorisation, authentication and TLS encryption to prevent unauthorised access
and prevent data interception or tampering.

Additionally, when communicating with any controller, JAAS uses token based authorisation whilst
additionally acting as the IdP (Identity Provider) for said tokens.

## Access control and authentication

JAAS provides an abstraction layer of access control on top of Juju. JAAS does this by backing its users
with an IdP (Identity Provider). Various identity providers can be used for this purpose (e.g. Google or Microsoft).
We recommend the [Canonical identity platform](https://charmhub.io/topics/canonical-identity-platform) as the preferred IdP for JAAS. The IdP will handle user
authentication on behalf of JAAS using OAuth 2.0 and OIDC. For authorisation, JAAS provides this by means
of tags and ReBAC (Relation-Based Access Control).

See the following pages for more details on how JAAS provides {ref}`jaas-authentication` and {ref}`jaas-authorization`.

## Auditing and logging

JAAS provides audit logs of all access to each model managed by JAAS, including information on which user
performed the action. Currently, JAAS does not provide a way to view the logs of applications within models.

## Model and user isolation

JAAS, like Juju, enables users with the correct cloud credentials associated with their user
to create models [under that clouds controller] for different applications, services, and
environments via the JAAS controller.

## Secure storage and handling of secrets

User provided cloud credentials are stored securely and safely within a secret vault. It is
essential to handle these secrets securely to prevent unauthorised access and data breaches
to the users cloud.
