// Copyright 2024 Canonical.
package middleware

import (
	"net/http"

	cofga "github.com/canonical/ofga"
	"github.com/go-chi/chi/v5"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/jujuapi"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
)

// AuthorizeUserForModelAccess extract the user from the context, and checks for permission on the model uuid extracted from the path.
func AuthorizeUserForModelAccess(next http.Handler, jimm jujuapi.JIMM, accessNeeded cofga.Relation) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user, err := GetUserFromContext(ctx)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		modelUUID := chi.URLParam(r, "uuid")
		if modelUUID == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("cannot find uuid in URL path"))
			return
		}
		modelTag := names.NewModelTag(modelUUID)
		switch accessNeeded {
		case ofganames.ReaderRelation:
			ok, err := user.IsModelReader(ctx, modelTag)
			if !ok || err != nil {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("no access to the resource"))
				return
			}
		case ofganames.WriterRelation:
			ok, err := user.IsModelWriter(ctx, modelTag)
			if !ok || err != nil {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("no access to the resource"))
				return
			}
		default:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("no access to the resource"))
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
