// Copyright 2016 Canonical Ltd.

package jujuapi

// pinger implements the Pinger facade.
type pinger struct{}

// Ping implements the Pinger facade's Ping method. It doesn't do
// anything.
func (p pinger) Ping() {}
