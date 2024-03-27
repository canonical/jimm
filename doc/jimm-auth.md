# OAuth, JIMM and OIDC


## Introduction
JIMM has introduced OAuth for federated authenticated, i.e., the ability to sign in via an external identity provider. The flow used is [authorisation code](https://auth0.com/docs/get-started/authentication-and-authorization-flow/authorization-code-flow). On top of this, JIMM now uses (OpenID Connect)[https://www.microsoft.com/en-us/security/business/security-101/what-is-openid-connect-oidc#:~:text=and%20use%20cases-,OpenID%20Connect%20(OIDC)%20defined,in%20to%20access%20digital%20services.].

To perform a login against JIMM using the authorisation code flow from a browser, there are 4 HTTP endpoints available and 1 websocket facade call. 

## Performing a login (HTTP)
### HTTP /auth/login GET
This will perform a a temporary redirect (307) to the /auth endpoint of JAAS' OAuth capable IdP server. The user will then be expected to login using any of the configured methods on the OAuth server, such as social sign in (e.g. Sign in with Google/Github/etc) or self service.

### HTTP /auth/callback REDIRECT
Upon a successful login, the OAuth server will redirect back to JIMM's callback endpoint.

This endpoint will do the following:
1. Authenticate the user with the OAuth server
2. Create a session for the user within JIMM's database
3. Create and return an encrypted cookie containing the session information
4. Redirect the user back to a configurable final redirect URL (likely the Juju dashboard)
5. Attempt to extract the email claim from the id_token
6. Create a session within JIMM's internal database and then attach an encrypted cookie containing the session identity ID to the response for the final redirect called "jimm-browser-session", finally, jimm redirect back the the configured final redirect URL (which is likely to be the Juju dashboard)

> Note: The cookie returned will have HTTP Only set to false, enabling SPA's (Single Page Application) to read the cookie.

After receiving the redirect from JIMM, the browser will now store the cookie and it can be used for the next steps. 

## Performing authentication (HTTP and WS)
### HTTP /auth/whoami GET
To confirm the identity that has been logged in from the cookie that has been returned in the final callback, the consumer will need to perform a get request to this endpoint. This endpoint will return (when a cookie can successfully be parsed into an application session and it is valid):
```json
{
    "display-name": "<string>",
	"email": "<string>"
}
```

In addition to this, the whoami endpoint will extend the users session by the configured max age field on the JIMM server, returning an updated cookie.

If no cookie is provided, a status Forbidden 403 will be returned, informing the consumer that they have no session cookie.

In the event of an internal server error, a status Internal Server Error 500 will be returned.

### WS /api and /api/{model id} WS PROTOCOL
The facade details to login are as follows:
- Facade name: `Admin`
- Version: `4 and above`
- Method name: `LoginWithSessionCookie`
- Parameters: `None`

The cookie header must be present on the initial request to open the websocket and must contain the cookie "jimm-browser-session", which holds the encrypted session identity that was returned in `/auth/callback`.

## Performing a logout (HTTP)
### /auth/logout GET
To logout, simply hit this endpoint. 

If no cookie is present, a status Forbidden 403 will be returned, informating the consumer that they have no session cookie.

In the event of an internal server error, a status Internal Server Error 500 will be returned.

Otherwise, a status OK 200 will be returned, which will reset the cookies max-age to -1, informing the browser to remove the session cookie immediately.

# Sessions
## The kind of sessions

### IdP Sessions
The IdP will hold a session for the authenticated user, meaning, that should another OAuth
flow be processed, if the user has already entered their credentials, and a session is active, they will not be expected to enter them again until the IdP session expires.

This means, should the user be redirected to the IdP's login page, they'll only have to perform a consent and not enter their credentials (if consent is enabled), and then they will be immediately redirected to the configured redirect URI callback.

### OAuth Sessions
OAuth sessions are often referred to as offline sessions, which directly relates to the use
of the [offline_access](https://openid.net/specs/openid-connect-core-1_0.html#OfflineAccess) scope. The access token expiry is used to determine if an OAuth session is currently active, and to be refreshed is handled by the IdP's offline_access idle timeout and/or expiry. If the refresh tokens idle timeout is reached, the token is revoked and any existing access tokens are also revoked.

For an example of how keycloak handles this, see [here](https://wjw465150.gitbooks.io/keycloak-documentation/content/server_admin/topics/sessions/offline.html). 

### Application Sessions
Application sessions are sessions between the client and the application, they may use means such as a JWT, cookie or some other means to authenticate the user for some period of time. The refreshing of these sessions is dependent on the OAuth session, and whether it is still valid.

