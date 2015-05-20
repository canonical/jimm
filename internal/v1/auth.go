package v1

import (
	"net/http"
	"strings"

	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/CanonicalLtd/jem/params"
)

const (
	usernameAttr = "username"
	groupsAttr   = "groups"
)

// authorization conatains authorization information extracted from an HTTP request.
// The zero value for a authorization contains no privileges.
type authorization struct {
	username string
	groups   []string
}

// checkRequest checks for any authorization tokens in the request and returns any
// found as an authorization. If no suitable credentials are found, or an error occurs,
// then a zero valued authorization is returned.
func (h *Handler) checkRequest(req *http.Request) (authorization, error) {
	attrMap, verr := httpbakery.CheckRequest(h.jem.Bakery, req, nil, checkers.New())
	if verr == nil {
		return authorization{
			username: attrMap[usernameAttr],
			groups:   strings.Fields(attrMap[groupsAttr]),
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
				Location:  h.config.IdentityLocation,
				Condition: "is-authenticated-user",
			},
			usernameAttr,
			groupsAttr,
		),
	})
}

func (h *Handler) isAdmin() bool {
	if h.auth.username == h.config.StateServerAdmin {
		return true
	}
	for _, g := range h.auth.groups {
		if g == h.config.StateServerAdmin {
			return true
		}
	}
	return false
}
