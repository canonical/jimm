# OAuth, JIMM and OIDC


## Introduction
To perform a login against JIMM using the authorisation code flow from a browser, there are 4 HTTP endpoints available and 1 websocket facade call. 

## Performing a login (HTTP)
### HTTP /auth/login GET
This will perform a a temporary redirect (307) to a preconfigured /auth endpoint of the configured OAuth capable IdP's server. The user will then be expected to login using any of the configured methods on the OAuth server, such as social sign in or self service.

### HTTP /auth/callback REDIRECT
Upon a successful login, the correctly configured IdP's OAuth server will redirect back to JIMM's callback endpoint.

This endpoint will do the following:
1. Extract the authorisation code
2. Exchange the authorisation code for an OAuth token
3. Attempt to extract an id_token (if the openid scope is not provided, this will fail and the login will be rejected)
4. Attempt to verify the id_token 
5. Attempt to extract the email claim from the id_token
6. Create a session within JIMM's internal database and then attach an encrypted cookie containing the session identity ID to the response for the final redirect called "jimm-browser-session", finally, jimm redirect back the the configured final redirect URL (which is likely to be the Juju dashboard)

> Note: The cookie returned will have HTTP Only set to false, enabling SPA's to read the cookie.

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

## How do I configure the session length?
The session length is dependent on the refresh tokens maximum age, which is handled at the IdP level called [offline_access](https://openid.net/specs/openid-connect-core-1_0.html#OfflineAccess). This maximum idle length will be handled at the IdP layer, meaning, if the idle is reached, the refresh session is removed. For an example of how keycloak handles this, see [here](https://wjw465150.gitbooks.io/keycloak-documentation/content/server_admin/topics/sessions/offline.html). 