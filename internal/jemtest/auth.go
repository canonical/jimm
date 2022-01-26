// Copyright 2016 Canonical Ltd.

package jemtest

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
)

const ControllerAdmin = "controller-admin"

var (
	Alice   = NewIdentity("alice", ControllerAdmin)
	Bob     = NewIdentity("bob", "bob-group")
	Charlie = NewIdentity("charlie")
	User1   = testIdentityWithDomain([]string{"user1"})
)

// NewIdentity returns an identity with the given ID that is a member of
// the given groups.
func NewIdentity(id string, groups ...string) identchecker.ACLIdentity {
	return testIdentity(append([]string{id}, groups...))
}

// A testIdentity is an identity for use in tests.
type testIdentity []string

// Allow implements identchecker.ACLIdentity.
func (i testIdentity) Allow(_ context.Context, acl []string) (bool, error) {
	for _, g := range acl {
		if g == "everyone" {
			return true, nil
		}
		for _, allowg := range i {
			if allowg == g {
				return true, nil
			}
		}
	}
	return false, nil
}

// Id implements identchecker.Identity.
func (i testIdentity) Id() string {
	return i[0]
}

// Domain implements identchecker.Identity.
func (i testIdentity) Domain() string {
	return ""
}

// A testIdentityWithDomain is an identity for use in tests.
type testIdentityWithDomain []string

// Allow implements identchecker.ACLIdentity.
func (i testIdentityWithDomain) Allow(_ context.Context, acl []string) (bool, error) {
	for _, g := range acl {
		if g == "everyone" {
			return true, nil
		}
		for _, allowg := range i {
			if allowg == g {
				return true, nil
			}
		}
	}
	return false, nil
}

// Id implements identchecker.Identity.
func (i testIdentityWithDomain) Id() string {
	return i[0]
}

// Domain implements identchecker.Identity.
func (i testIdentityWithDomain) Domain() string {
	return "test-domain"
}
