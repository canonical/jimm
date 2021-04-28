// Copyright 2017 Canonical Ltd.

package jujuapi

import (
	jujuparams "github.com/juju/juju/apiserver/params"

	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
)

// modelRoot is the root for endpoints served on model connections.
type modelRoot struct {
	rpc.Root

	jimm         *jimm.JIMM
	uuid         string
	redirectInfo jujuparams.RedirectInfoResult
	heartMonitor heartMonitor
}

func newModelRoot(j *jimm.JIMM, hm heartMonitor, uuid string) *modelRoot {
	r := &modelRoot{
		jimm:         j,
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
