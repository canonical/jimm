package jimmtest

import (
	"time"

	"github.com/canonical/jimm"
	"github.com/coreos/go-oidc/v3/oidc"
)

// NewTestJimmParams returns a set of JIMM params with sensible defaults
// for tests. A test can override any parameter that it needs.
// Note that NewTestJimmParams will create an empty test database.
func NewTestJimmParams(t Tester) jimm.Params {
	return jimm.Params{
		DSN:            CreateEmptyDatabase(t),
		ControllerUUID: "6acf4fd8-32d6-49ea-b4eb-dcb9d1590c11",
		PrivateKey:     "ly/dzsI9Nt/4JxUILQeAX79qZ4mygDiuYGqc2ZEiDEc=",
		PublicKey:      "izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk=",
		OAuthAuthenticatorParams: jimm.OAuthAuthenticatorParams{
			IssuerURL:           "http://localhost:8082/realms/jimm",
			ClientID:            "jimm-device",
			Scopes:              []string{oidc.ScopeOpenID, "profile", "email"},
			SessionTokenExpiry:  time.Duration(time.Hour),
			SessionCookieMaxAge: 60,
			SessionSecretKey:    "test-secret",
		},
		DashboardFinalRedirectURL: "dashboard-url",
	}
}
