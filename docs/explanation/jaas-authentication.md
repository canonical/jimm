(jaas-authentication)=
# Authentication

As a brief refresher, authentication refers to the process of proving something to be true, in this case proving that
the user logging in is who they say they are.

This is one of the key features of JAAS. Where Juju controllers implement login via commonly understood username/password authentication,
JAAS uses [OAuth 2.0](https://auth0.com/intro-to-iam/what-is-oauth-2) and [OIDC](https://developer.okta.com/blog/2019/10/21/illustrated-guide-to-oauth-and-oidc).
While a full explanation of OAuth and OIDC are out of the scope of this document, you are likely already familiar with
the benefits of these standards when you log into various services across the internet.

These standards define how services can access your resources on your behalf and how services can authenticate your identity.
When logging into a web application that employs OIDC you will commonly be asked to login via a different website or provider,
like your email or social media provider and this information is then securely passed onto the original application.

## Users

Users in Juju and JAAS have important differences.

When operating a standalone Juju controller, users are created and stored locally in the controller's database.
These are referred to as "local users", the most common local user is the initial "admin" user created on bootstrap.

In JAAS, local users are replaced by "external users" i.e., users that come from an external
identity provider, which can represent people or service accounts. In JAAS there is no initial administrator, instead
an initial admin must be {doc}`provisioned <../howto/manage-your-jaas-deployment>`.

The username for external users in JAAS is either an email address or a service account ID with the suffix `@serviceaccount`.

## Login providers

Because JAAS uses the OAuth 2.0/OIDC standard, theoretically various providers can be connected to JAAS and used as a login provider.
However, due to the varying security practices and slight deviations from the standard, not all providers are supported with JAAS.

Officially, JAAS supports [Ory Hydra](https://www.ory.sh/hydra/), a cloud native OAuth 2.0 and OIDC server. This is a key component of
the [Canonical identity platform](https://charmhub.io/topics/canonical-identity-platform) which not only provides a standards compliant OAuth/OIDC server but also allows you to configure
social sign-on via other OIDC compliant identity providers (e.g. Azure AD, Google, Okta, etc.).


## Authentication methods

JAAS offers multiple OAuth 2.0 flows (a sequence of steps to login). Each of which is referred to as a **grant type**.

**Authorisation Code grant**: This flow is the most common and used primarily by web applications. You will encounter this flow with JAAS when using
the Juju dashboard. The login process will redirect your browser to JAAS' identity provider and ask you to login before redirecting you
to the dashboard. At this point you have been authenticated and can use your resources through the graphical interface.

**Device Code grant**: You will encounter this flow when using the Juju CLI with JAAS. If you are logging in for the first time or if your
session has expired you will be prompted with URL and unique code. Navigating to the page will ask you to login and provide the code.
During this time the CLI will continually ping the server until authentication is complete.

## Sessions

A brief mention on sessions is also important in the context of authentication. While JAAS authenticates a user by communicating with
an external identity provider, this is neither performant nor would make a great user experience if a user were asked to log in after each interaction.

To solve this, JAAS also provides users with their own application sessions. Depending on your authentication flow, your session with
JAAS will last a varying amount of time until you are asked to log in again. This is a configurable option to cater for different
organisational needs.
