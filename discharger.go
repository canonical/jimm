// Copyright 2023 Canonical Ltd.

package jimm

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	jjmacaroon "github.com/juju/juju/core/macaroon"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/pkg/names"
)

var defaultDischargeExpiry = 15 * time.Minute

func newMacaroonDischarger(p Params, db *db.Database, ofgaClient *openfga.OFGAClient) (*macaroonDischarger, error) {
	var kp bakery.KeyPair
	if p.PublicKey == "" || p.PrivateKey == "" {
		generatedKP, err := bakery.GenerateKey()
		if err != nil {
			return nil, errors.E(err, "failed to generate a bakery keypair")
		}
		kp = *generatedKP
	} else {
		if err := kp.Private.UnmarshalText([]byte(p.PrivateKey)); err != nil {
			return nil, errors.E(err, "cannot unmarshal private key")
		}
		if err := kp.Public.UnmarshalText([]byte(p.PublicKey)); err != nil {
			return nil, errors.E(err, "cannot unmarshal public key")
		}
	}

	checker := checkers.New(jjmacaroon.MacaroonNamespace)
	b := bakery.New(
		bakery.BakeryParams{
			Checker: checker,
			RootKeyStore: dbrootkeystore.NewRootKeys(100, nil).NewStore(
				db,
				dbrootkeystore.Policy{
					ExpiryDuration: p.MacaroonExpiryDuration,
				},
			),
			Key:      &kp,
			Location: "jimm " + p.ControllerUUID,
		},
	)

	return &macaroonDischarger{
		ofgaClient: ofgaClient,
		bakery:     b,
		kp:         kp,
	}, nil
}

type macaroonDischarger struct {
	ofgaClient *openfga.OFGAClient
	bakery     *bakery.Bakery
	kp         bakery.KeyPair
}

// thirdPartyCaveatCheckerFunction returns a function that
// checks third party caveats addressed to this service.
// Caveat format is:
//
//	is-<relation name> <user tag> <resource tag>
//
// Examples of caveats are:
//
//	is-reader <user tag> <offer tag containing uuid>
//	is-consumer <user tag> <offer tag containing uuid>
//	is-administrator <user tag> <offer tag containing uuid>
//	is-reader <user tag> <model tag containing uuid>
//	is-writer <user tag> <model tag containing uuid>
//	is-admininistrator <user tag> <model tag containing uuid>
//	is-admininistrator <user tag> <controller tag containing uuid>
//
// The discharged macaroon will contain a time-before first party caveat and
// a declared caveat declaring relation to the required entity in form of:
//
//	<relation> <entity tag>
//
// Example:
//  1. if the third party caveat condition is:
//     is-reader <user tag> <offer tag containing uuid>
//     the declared caveat will contain
//     reader <offer tag>
//  2. if the third party caveat condition is:
//     is-writer <user tag> <model tag containing uuid>
//     the declared caveat will contain
//     writer <model tag>
func (md *macaroonDischarger) checkThirdPartyCaveat(ctx context.Context, req *http.Request, cavInfo *bakery.ThirdPartyCaveatInfo, _ *httpbakery.DischargeToken) ([]checkers.Caveat, error) {
	caveatTokens := strings.Split(string(cavInfo.Condition), " ")
	if len(caveatTokens) != 3 {
		zapctx.Error(ctx, "caveat token length incorrect", zap.Int("length", len(caveatTokens)))
		return nil, checkers.ErrCaveatNotRecognized
	}
	relationString := caveatTokens[0]
	userTagString := caveatTokens[1]
	objectTagString := caveatTokens[2]

	if !strings.HasPrefix(relationString, "is-") {
		zapctx.Error(ctx, "caveat token relation string missing prefix")
		return nil, checkers.ErrCaveatNotRecognized
	}
	relationString = strings.TrimPrefix(relationString, "is-")
	relation, err := ofganames.ParseRelation(relationString)
	if err != nil {
		zapctx.Error(ctx, "caveat token relation invalid", zap.Error(err))
		return nil, checkers.ErrCaveatNotRecognized
	}

	userTag, err := names.ParseUserTag(userTagString)
	if err != nil {
		zapctx.Error(ctx, "failed to parse caveat user tag", zap.Error(err))
		return nil, checkers.ErrCaveatNotRecognized
	}

	objectTag, err := jimmnames.ParseTag(objectTagString)
	if err != nil {
		zapctx.Error(ctx, "failed to parse caveat object tag", zap.Error(err))
		return nil, checkers.ErrCaveatNotRecognized
	}

	user := openfga.NewUser(
		&dbmodel.User{
			Username: userTag.Id(),
		},
		md.ofgaClient,
	)

	allowed, err := openfga.CheckRelation(ctx, user, objectTag, relation)
	if err != nil {
		zapctx.Error(ctx, "failed to check request caveat relation", zap.Error(err))
		return nil, errors.E(err)
	}

	if allowed {
		return []checkers.Caveat{
			checkers.DeclaredCaveat(relationString, objectTagString),
			checkers.TimeBeforeCaveat(time.Now().Add(defaultDischargeExpiry)),
		}, nil
	}
	zapctx.Debug(ctx, "macaroon dishcharge denied", zap.String("user", user.Username), zap.String("object", objectTag.Id()))
	return nil, httpbakery.ErrPermissionDenied
}
