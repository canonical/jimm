// Copyright 2024 Canonical.

package jujuapi

func init() {
	facadeInit["Pinger"] = func(_ *controllerRoot) []int {
		// The ping method is is registered on the unauthenticated
		// connection there is no need to register it again here.
		return []int{1}
	}
}

// Ping implemets the Pinger facade's Ping method on controller API
// connections.
func (r *controllerRoot) Ping() {
	r.pingF()
}
