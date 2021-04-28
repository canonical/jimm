// Copyright 2021 CanonicalLtd.

package auth

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	bakeryv2 "gopkg.in/macaroon-bakery.v2/bakery"
	identcheckerv2 "gopkg.in/macaroon-bakery.v2/bakery/identchecker"
)

// IdentityClientV3 "upgrades" a v2 IdentityClient to a v3 one.
type IdentityClientV3 struct {
	IdentityClient identcheckerv2.IdentityClient
}

// IdentityFromContext implements IdentityClient.
func (c IdentityClientV3) IdentityFromContext(ctx context.Context) (identchecker.Identity, []checkers.Caveat, error) {
	var id identchecker.Identity
	var cavs []checkers.Caveat
	id2, cavs2, err := c.IdentityClient.IdentityFromContext(ctx)
	if id2 != nil {
		id = id2.(identchecker.Identity)
	}
	if cavs2 != nil {
		cavs = make([]checkers.Caveat, len(cavs2))
	}
	for i, cav2 := range cavs2 {
		cavs[i] = checkers.Caveat{
			Condition: cav2.Condition,
			Namespace: cav2.Namespace,
			Location:  cav2.Location,
		}
	}
	return id, cavs, err
}

// DeclaredIdentity implements IdentityClient.
func (c IdentityClientV3) DeclaredIdentity(ctx context.Context, declared map[string]string) (identchecker.Identity, error) {
	var id identchecker.Identity
	id2, err := c.IdentityClient.DeclaredIdentity(ctx, declared)
	if id2 != nil {
		id = id2.(identchecker.Identity)
	}
	return id, err
}

// A ThirdPartyLocatorV3 adapts a v2 ThirdPartyLocator to a v3 one.
type ThirdPartyLocatorV3 struct {
	ThirdPartyLocator bakeryv2.ThirdPartyLocator
}

// ThirdPartyInfo implements ThirdPartyLocator.
func (l ThirdPartyLocatorV3) ThirdPartyInfo(ctx context.Context, loc string) (bakery.ThirdPartyInfo, error) {
	var tpi bakery.ThirdPartyInfo
	tpi2, err := l.ThirdPartyLocator.ThirdPartyInfo(ctx, loc)
	tpi.PublicKey.Key = bakery.Key(tpi2.PublicKey.Key)
	tpi.Version = bakery.Version(tpi.Version)
	return tpi, err
}
