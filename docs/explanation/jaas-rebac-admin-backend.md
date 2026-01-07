(jaas-rebac-admin-backend)=
# ReBAC admin backend

The ReBAC Admin API is a REST API that provides various endpoints to query or
manipulate relationships in JAAS ReBAC authorisation model.

## OpenAPI specification

The OpenAPI spec can be found at `https://<jimm-deployment>/rebac/v1/swagger.json`

## Authentication

These endpoints are meant to be called from a web browser, therefore the authentication is handled via Cookies.

## JAAS Implementation

JAAS implements a subset of the operations described in the OpenAPI spec.

| Status | Entities | Notes|
|-|-|-|
| ✅  | `entitlements` ||
| ✅  | `capabilities` ||
| ✅  | `groups` ||
| ✅  | `resources` ||
| ✅  | `roles`      ||
| 🟡  | `identities`  |   No support for creation, update and deletion.|
| ❌  | `authentication` | No support for identity provider management.|
