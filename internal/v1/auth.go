package v1

import (
	"net/http"

	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

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

// checkIsUser checks whether the current user can
// act as the given name.
func (h *Handler) checkIsUser(user params.User) error {
	return h.checkACL([]string{string(user)})
}

// checkACL checks whether the current user is
// allowed to access an entity with the given ACL.
// It returns params.ErrUnauthorized if not.
func (h *Handler) checkACL(acl []string) error {
	ok, err := h.jem.PermChecker.Allow(h.auth.username, acl)
	if err != nil {
		return errgo.Notef(err, "cannot check permissions")
	}
	if !ok {
		return params.ErrUnauthorized
	}
	return nil
}

// checkCanRead checks whether the current user is
// allowed to read the given entity. The owner
// is always allowed to access an entity, regardless
// of its ACL.
func (h *Handler) checkCanRead(e aclEntity) error {
	acl := append([]string{string(e.Owner())}, e.GetACL().Read...)
	return h.checkACL(acl)
}

// checkReadACL checks that the entity with the given path in the
// given collection can be read by the currently authenticated user.
func (h *Handler) checkReadACL(coll *mgo.Collection, path params.EntityPath) error {
	// The user can always access their own entities.
	if err := h.checkIsUser(path.User); err == nil {
		return nil
	}
	acl, err := h.getACL(coll, path)
	if errgo.Cause(err) == params.ErrNotFound {
		// The document is not found - and we've already checked
		// that the currently authenticated user cannot speak for
		// path.User, so return an unauthorized error to stop
		// people probing for the existence of other people's entities.
		return params.ErrUnauthorized
	}
	if err := h.checkACL(acl.Read); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return nil
}

var selectACL = bson.D{{"acl", 1}}

func (h *Handler) getACL(coll *mgo.Collection, path params.EntityPath) (params.ACL, error) {
	var doc struct {
		ACL params.ACL
	}
	if err := coll.FindId(path.String()).Select(selectACL).One(&doc); err != nil {
		if err == mgo.ErrNotFound {
			err = params.ErrNotFound
		}
		return params.ACL{}, errgo.Mask(err, errgo.Is(mgo.ErrNotFound))
	}
	return doc.ACL, nil
}

// listIter returns an iterator that iterates over items
// in the given iterator, skipping over those that
// the currently logged in user does not have permissions
// to see.
func (h *Handler) listIter(iter *mgo.Iter) *listIter {
	return &listIter{
		h:    h,
		Iter: iter,
	}
}

type listIter struct {
	h *Handler
	*mgo.Iter
	err error
}

// aclEntity represents an entity with access permissions.
type aclEntity interface {
	GetACL() params.ACL
	Owner() params.User
}

// Next reads the next item from the iterator into the given
// item and returns whether it has done so.
func (iter *listIter) Next(item aclEntity) bool {
	if iter.err != nil {
		return false
	}
	for iter.Iter.Next(item) {
		if err := iter.h.checkCanRead(item); err != nil {
			if errgo.Cause(err) == params.ErrUnauthorized {
				// No permissions to look at the entity, so don't include
				// it in the results.
				continue
			}
			iter.err = errgo.Mask(err)
			iter.Iter.Close()
			return false
		}
		return true
	}
	return false
}

func (iter *listIter) Err() error {
	if iter.err != nil {
		return iter.err
	}
	return iter.Iter.Err()
}
