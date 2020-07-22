// Copyright 2017 Canonical Ltd.

package jujuapi

import (
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
)

// modelRoot is the root for endpoints served on model connections.
type modelRoot struct {
	rpc.Root

	jem          *jem.JEM
	uuid         string
	model        *mongodoc.Model
	controller   *mongodoc.Controller
	heartMonitor heartMonitor
}

func newModelRoot(jem *jem.JEM, hm heartMonitor, uuid string) *modelRoot {
	r := &modelRoot{
		jem:          jem,
		uuid:         uuid,
		heartMonitor: hm,
	}
	r.AddMethod("Admin", 1, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 2, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 3, "Login", rpc.Method(r.Login))
	r.AddMethod("Admin", 3, "RedirectInfo", rpc.Method(r.RedirectInfo))
	r.AddMethod("Pinger", 1, "Ping", rpc.Method(ping))
	return r
}
