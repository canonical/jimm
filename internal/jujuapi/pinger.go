// Copyright 2016 Canonical Ltd.

package jujuapi

func init() {
	facadeInit["Pinger"] = func(_ *controllerRoot) []int {
		// The ping method is is regestered on the unauthenticated
		// connection there is no need to register it again here.
		return []int{1}
	}
}

// ping implements the Pinger facade's Ping method. It doesn't do
// anything.
func ping() {}
