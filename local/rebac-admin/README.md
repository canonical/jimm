# QA ReBAC Admin Backend

To QA locally the ReBAC Admin Backend you need to:

## Change the rebac authentication middleware at `internal/middleware/authn.go` to:
```go
func AuthenticateRebac(baseURL string, next http.Handler, jimm JIMMAuthner) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		relativePath, _ := strings.CutPrefix(r.URL.Path, baseURL)
		if _, found := unauthenticatedEndpoints[relativePath]; found {
			next.ServeHTTP(w, r)
			return
		}
		identity := "admin@canonical.com"
		user, err := jimm.UserLogin(ctx, identity)
		if err != nil {
			zapctx.Error(ctx, "failed to get openfga user", zap.Error(err))
			http.Error(w, "internal authentication error", http.StatusInternalServerError)
			return
		}
		user.JimmAdmin = true
		if !user.JimmAdmin {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("user is not an admin"))
			return
		}

		ctx = rebac_handlers.ContextWithIdentity(ctx, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
```
This change will create and authenticate an admin user used by the QA script.

## Start JIMM

Follow [local/README.md](../README.md)

## Launch the script

`./local/rebac-admin/qa.sh --bail-on-error --cleanup`

This will:
- create random groups
- add identity to it
- add some entitlements to groups and identities
- remove entitlements
- remove groups


> The script is a fork from [rebac-admin-ui-handlers](https://github.com/canonical/rebac-admin-ui-handlers/blob/main/_example/test.sh), adapted for JIMM's deployment. Some parts of this script have been left commented out to show they are not implemented yet.