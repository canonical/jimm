// Copyright 2018 Canonical Ltd.

package admin

import (
	"context"
	"net/http"

	"github.com/juju/aclstore"
	"github.com/juju/httprequest"
	"github.com/juju/simplekv/mgosimplekv"
	"github.com/julienschmidt/httprouter"
	errgo "gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/auth"
	"github.com/CanonicalLtd/jem/internal/jemerror"
	"github.com/CanonicalLtd/jem/internal/jemserver"
	"github.com/CanonicalLtd/jem/params"
)

// NewAPIHandler creates a new admin API handler.
func NewAPIHandler(ctx context.Context, params jemserver.HandlerParams) ([]httprequest.Handler, error) {
	m, err := aclstore.NewManager(ctx, aclstore.Params{
		Store:    params.ACLStore,
		RootPath: "/admin/acls",
		Authenticate: func(ctx context.Context, w http.ResponseWriter, req *http.Request) (aclstore.Identity, error) {
			authenticator := params.AuthenticatorPool.Authenticator(ctx)
			defer authenticator.Close()
			ctx, err := authenticator.AuthenticateRequest(ctx, req)
			if err != nil {
				status, body := jemerror.Mapper(err)
				httprequest.WriteJSON(w, status, body)
				return nil, errgo.Mask(err, errgo.Any)
			}
			return identity{ctx}, nil
		},
		InitialAdminUsers: []string{string(params.ControllerAdmin)},
	})
	if err != nil {
		return nil, errgo.Mask(err)
	}
	f := func(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
		ctx, close := params.SessionPool.ContextWithSession(req.Context())
		defer close()
		s := params.SessionPool.Session(ctx)
		defer s.Close()
		ctx = mgosimplekv.ContextWithSession(ctx, s)
		req = req.WithContext(ctx)
		m.ServeHTTP(w, req)
	}
	return []httprequest.Handler{
		{"GET", "/admin/acls/*path", f},
		{"POST", "/admin/acls/*path", f},
		{"PUT", "/admin/acls/*path", f},
	}, nil
}

type identity struct {
	ctx context.Context
}

func (i identity) Allow(_ context.Context, acl []string) (bool, error) {
	if err := auth.CheckACL(i.ctx, acl); err != nil {
		if errgo.Cause(err) == params.ErrUnauthorized {
			return false, nil
		}
		return false, errgo.Mask(err)
	}
	return true, nil
}
