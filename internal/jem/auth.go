package jem

import (
	"net/http"

	"github.com/juju/utils"
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

// Authorization contains authorization information extracted from an HTTP request.
// The zero value contains no privileges.
type Authorization struct {
	Username string
}

// Authenticate checks for any authorization tokens in the request and sets
// j.Auth to any authenticated credentials found. If no suitable
// credentials are found, or an error occurs, j.Auth is zeroed.
func (j *JEM) Authenticate(req *http.Request) error {
	attrMap, verr := httpbakery.CheckRequest(j.Bakery, req, nil, checkers.New())
	if verr == nil {
		j.Auth = Authorization{
			Username: attrMap[usernameAttr],
		}
		return nil
	}
	j.Auth = Authorization{}
	if _, ok := errgo.Cause(verr).(*bakery.VerificationError); !ok {
		return errgo.Mask(verr, errgo.Is(params.ErrUnauthorized))
	}
	// Macaroon verification failed: mint a new macaroon.
	m, err := j.NewMacaroon()
	if err != nil {
		return errgo.Notef(err, "cannot mint macaroon")
	}
	// Request that this macaroon be supplied for all requests
	// to the whole handler. We use a relative path because
	// the JEM server is conventionally under an external
	// root path, with other services also under the same
	// externally visible host name, and we don't want our
	// cookies to be presented to those services.
	cookiePath := "/"
	if p, err := utils.RelativeURLPath(req.RequestURI, "/"); err != nil {
		// Should never happen, as RequestURI should always be absolute.
		logger.Infof("cannot make relative URL from %v", req.RequestURI)
	} else {
		cookiePath = p
	}
	dischargeErr := httpbakery.NewDischargeRequiredErrorForRequest(m, cookiePath, verr, req)
	dischargeErr.(*httpbakery.Error).Info.CookieNameSuffix = "authn"
	return dischargeErr
}

// NewMacaroon returns a macaroon that, when discharged,
// will allow access to JEM.
func (j *JEM) NewMacaroon() (*macaroon.Macaroon, error) {
	return j.Bakery.NewMacaroon("", nil, []checkers.Caveat{
		checkers.NeedDeclaredCaveat(
			checkers.Caveat{
				Location:  j.pool.config.IdentityLocation,
				Condition: "is-authenticated-user",
			},
			usernameAttr,
		),
	})
}

// CheckIsAdmin checks that the currently authenticated user has
// administrator privileges.
func (j *JEM) CheckIsAdmin() error {
	return j.CheckIsUser(params.User(j.pool.config.ControllerAdmin))
}

// CheckIsUser checks whether the currently authenticated user can
// act as the given name.
func (j *JEM) CheckIsUser(user params.User) error {
	return j.CheckACL([]string{string(user)})
}

// CheckACL checks whether the currently authenticated user is
// allowed to access an entity with the given ACL.
// It returns params.ErrUnauthorized if not.
func (j *JEM) CheckACL(acl []string) error {
	ok, err := j.PermChecker.Allow(j.Auth.Username, acl)
	if err != nil {
		return errgo.Notef(err, "cannot check permissions")
	}
	if !ok {
		return params.ErrUnauthorized
	}
	return nil
}

// CheckCanRead checks whether the current user is
// allowed to read the given entity. The owner
// is always allowed to access an entity, regardless
// of its ACL.
func (j *JEM) CheckCanRead(e ACLEntity) error {
	acl := append([]string{string(e.Owner())}, e.GetACL().Read...)
	return j.CheckACL(acl)
}

// CheckReadACL checks that the entity with the given path in the
// given collection can be read by the currently authenticated user.
func (j *JEM) CheckReadACL(coll *mgo.Collection, path params.EntityPath) error {
	// The user can always access their own entities.
	if err := j.CheckIsUser(path.User); err == nil {
		return nil
	}
	acl, err := j.GetACL(coll, path)
	if errgo.Cause(err) == params.ErrNotFound {
		// The document is not found - and we've already checked
		// that the currently authenticated user cannot speak for
		// path.User, so return an unauthorized error to stop
		// people probing for the existence of other people's entities.
		return params.ErrUnauthorized
	}
	if err := j.CheckACL(acl.Read); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return nil
}

var selectACL = bson.D{{"acl", 1}}

// GetACL retrieves the ACL for the document at path in coll. If the
// document is not found, the returned error will have the cause
// params.ErrNotFound.
func (j *JEM) GetACL(coll *mgo.Collection, path params.EntityPath) (params.ACL, error) {
	var doc struct {
		ACL params.ACL
	}
	if err := coll.FindId(path.String()).Select(selectACL).One(&doc); err != nil {
		if err == mgo.ErrNotFound {
			err = params.ErrNotFound
		}
		return params.ACL{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return doc.ACL, nil
}

// CanReadIter returns an iterator that iterates over items
// in the given iterator, returning only those
// that  the currently logged in user has permission
// to see.
//
// The API matches that of mgo.Iter.
func (j *JEM) CanReadIter(iter *mgo.Iter) *CanReadIter {
	return &CanReadIter{
		jem:  j,
		iter: iter,
	}
}

// CanReadIter represents an iterator that returns only items
// that the currently authenticated user has read access to.
type CanReadIter struct {
	jem  *JEM
	iter *mgo.Iter
	err  error
}

// ACLEntity represents a mongo entity with access permissions.
type ACLEntity interface {
	GetACL() params.ACL
	Owner() params.User
}

// Next reads the next item from the iterator into the given
// item and returns whether it has done so.
func (iter *CanReadIter) Next(item ACLEntity) bool {
	if iter.err != nil {
		return false
	}
	for iter.iter.Next(item) {
		if err := iter.jem.CheckCanRead(item); err != nil {
			if errgo.Cause(err) == params.ErrUnauthorized {
				// No permissions to look at the entity, so don't include
				// it in the results.
				continue
			}
			iter.err = errgo.Mask(err)
			iter.iter.Close()
			return false
		}
		return true
	}
	return false
}

func (iter *CanReadIter) Close() error {
	iter.iter.Close()
	return iter.Err()
}

// Err returns any error encountered when iterating.
func (iter *CanReadIter) Err() error {
	if iter.err != nil {
		return iter.err
	}
	return iter.iter.Err()
}
