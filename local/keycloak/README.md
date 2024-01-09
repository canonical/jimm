# Keycloak

## Why Keycloak?
As of 9th Jan, 2024, the companies desired OAuth2.0 server does NOT support the device OAuth2.0 flow, and it is required for JIMM to migrate away from Macaroons. As such, for local development, Keycloak has been chosen as it supports the device flow out of the box, including ALL other OAuth2.0 standard and extension grants and flows.

## What is Keycloak?
Keycloak is an open-source identity and access management tool that supports standard IAM protocols such as OAuth 2.0, OpenID Connect, and SAML. It enables the creation of OAuth 2.0 clients for secure authorisation and authentication.

The key features in this local development environment we require are the user management and storage, and the OAuth2.0 server.

## What is a "Realm"?
A Keycloak realm is a security and administrative domain where users, applications, roles, and groups are managed. Realms are isolated from each other and can only manage and authenticate the users that they contain. Keycloak allows for the creation, storage, and management of multiple realms within a single deployment, providing a way to isolate and secure different sets of applications and users. The realm acts as a container for all the objects that make up your security domain. In our local development environment, we have a preconfigured realm that is imported on startup.

## What is a "Client"?
In Keycloak, a client represents an application that can request authentication or authorisation. Clients can be web applications, service accounts, or other types of applications that interact with Keycloak for security purposes. They are registered within a specific realm and can be configured to use different authentication methods, such as client ID and client secret, signed JWT, or other supported mechanisms. Additionally, clients can define roles specific to them and are associated with client scopes, which are useful for sharing common settings and requesting claims or roles based on scope parameters.

In the context of OpenID Connect, scopes are used to request specific sets of user details, such as name and email, during authentication. Each scope returns a set of user attributes, known as claims. The "openid" scope is required and indicates that an application intends to use the OpenID Connect protocol to verify a user's identity. Other scopes, such as "profile" and "email," allow applications to request additional user details. Claims are the assertions made about a subject, and scopes are groups of claims used for access control.

## How does Keycloak differ from Hydra and Kratos?
Keycloak is considered an all-in-one solution, where as Kratos is strictly an Identity provider and Hydra is an OAuth2.0 server AND provider. For all intents and purposes, they are the same, but some language is different.

Here are some synoynmous terms:
- Keycloak Users -> Kratos Identites
- Keycloak Clients (SAML, OIDC, JWT, etc.) -> Hydra OAuth2.0 Client
- Keycloak Role Mappings -> Hydra Trait Mapping
- Keycloak Realm -> Ory Project

