// Copyright 2016 Canonical Ltd.

package jujuapi

func init() {
	facadeInit["Pinger"] = func(_ *controllerRoot) []int {
		// The ping method is is registered on the unauthenticated
		// connection there is no need to register it again here.
		return []int{1}
	}
}

// ping implements the Pinger facade's Ping method. It doesn't do
// anything.
func ping() {}

// Ping implemets the Pinger facade's Ping method on controller API
// connections.
func (r *controllerRoot) Ping() {
	r.pingF()
}

// Ping implemets the Pinger facade's Ping method on model API connections.
func (r *modelRoot) Ping() {
	r.pingF()
}
