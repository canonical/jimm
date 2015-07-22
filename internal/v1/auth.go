package v1

import (
	"net/http"

	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/CanonicalLtd/jem/params"
)

const (
	usernameAttr = "username"
)

// authorization conatains authorization information extracted from an HTTP request.
// The zero value for a authorization contains no privileges.
type authorization struct {
	username string
}

// checkRequest checks for any authorization tokens in the request and returns any
// found as an authorization. If no suitable credentials are found, or an error occurs,
// then a zero valued authorization is returned.
func (h *Handler) checkRequest(req *http.Request) (authorization, error) {
	attrMap, verr := httpbakery.CheckRequest(h.jem.Bakery, req, nil, checkers.New())
	if verr == nil {
		return authorization{
			username: attrMap[usernameAttr],
		}, nil
	}
	if _, ok := errgo.Cause(verr).(*bakery.VerificationError); !ok {
		return authorization{}, errgo.Mask(verr, errgo.Is(params.ErrUnauthorized))
	}
	// Macaroon verification failed: mint a new macaroon.
	m, err := h.newMacaroon()
	if err != nil {
		return authorization{}, errgo.Notef(err, "cannot mint macaroon")
	}
	// Request that this macaroon be supplied for all requests
	// to the whole handler.
	// TODO use a relative URL here: router.RelativeURLPath(req.RequestURI, "/")
	cookiePath := "/"
	return authorization{}, httpbakery.NewDischargeRequiredError(m, cookiePath, verr)
}

func (h *Handler) newMacaroon() (*macaroon.Macaroon, error) {
	return h.jem.Bakery.NewMacaroon("", nil, []checkers.Caveat{
		checkers.NeedDeclaredCaveat(
			checkers.Caveat{
				Location:  h.config.IdentityLocation + "/v1/discharger",
				Condition: "is-authenticated-user",
			},
			usernameAttr,
		),
	})
}

func (h *Handler) checkIsAdmin() error {
	return h.checkIsUser(params.User(h.config.StateServerAdmin))
}

func (h *Handler) checkIsUser(name params.User) error {
	ok, err := h.jem.PermChecker.Allow(h.auth.username, []string{string(name)})
	if err != nil {
		return errgo.Notef(err, "cannot check permissions")
	}
	if !ok {
		return params.ErrUnauthorized
	}
	return nil
}
