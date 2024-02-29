// Copyright 2023 Canonical Ltd.

package discharger

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
	"github.com/juju/names/v5"
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

type MacaroonDischargerConfig struct {
	PublicKey              string
	PrivateKey             string
	MacaroonExpiryDuration time.Duration
	ControllerUUID         string
}

func NewMacaroonDischarger(cfg MacaroonDischargerConfig, db *db.Database, ofgaClient *openfga.OFGAClient) (*MacaroonDischarger, error) {
	var kp bakery.KeyPair
	if cfg.PublicKey == "" || cfg.PrivateKey == "" {
		generatedKP, err := bakery.GenerateKey()
		if err != nil {
			return nil, errors.E(err, "failed to generate a bakery keypair")
		}
		kp = *generatedKP
	} else {
		if err := kp.Private.UnmarshalText([]byte(cfg.PrivateKey)); err != nil {
			return nil, errors.E(err, "cannot unmarshal private key")
		}
		if err := kp.Public.UnmarshalText([]byte(cfg.PublicKey)); err != nil {
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
					ExpiryDuration: cfg.MacaroonExpiryDuration,
				},
			),
			Key:      &kp,
			Location: "jimm " + cfg.ControllerUUID,
		},
	)

	return &MacaroonDischarger{
		ofgaClient: ofgaClient,
		bakery:     b,
		kp:         kp,
	}, nil
}

type MyTest struct{}
type MacaroonDischarger struct {
	ofgaClient *openfga.OFGAClient
	bakery     *bakery.Bakery
	kp         bakery.KeyPair
}

// GetDischargerMux returns a mux that can handle macaroon bakery requests for the provided discharger.
func GetDischargerMux(MacaroonDischarger *MacaroonDischarger, rootPath string) *http.ServeMux {
	discharger := httpbakery.NewDischarger(
		httpbakery.DischargerParams{
			Key:     &MacaroonDischarger.kp,
			Checker: httpbakery.ThirdPartyCaveatCheckerFunc(MacaroonDischarger.CheckThirdPartyCaveat),
		},
	)
	dischargeMux := http.NewServeMux()
	discharger.AddMuxHandlers(dischargeMux, rootPath)

	return dischargeMux
}

// thirdPartyCaveatCheckerFunction returns a function that
// checks third party caveats addressed to this service.
// Caveat format is:
//
//	is-consumer <user tag> <offer uuid>
//
// The discharged macaroon will contain a time-before first party caveat and
// a declared caveat declaring offer uuid:
//
//	declared offer-uuid <offer uuid>
func (md *MacaroonDischarger) CheckThirdPartyCaveat(ctx context.Context, req *http.Request, cavInfo *bakery.ThirdPartyCaveatInfo, _ *httpbakery.DischargeToken) ([]checkers.Caveat, error) {
	caveatTokens := strings.Split(string(cavInfo.Condition), " ")
	if len(caveatTokens) != 3 {
		zapctx.Error(ctx, "caveat token length incorrect", zap.Int("length", len(caveatTokens)))
		return nil, checkers.ErrCaveatNotRecognized
	}
	relationString := caveatTokens[0]
	userTagString := caveatTokens[1]
	offerUUID := caveatTokens[2]

	if relationString != "is-consumer" {
		zapctx.Error(ctx, "unknown third party caveat", zap.String("condition", relationString))
		return nil, checkers.ErrCaveatNotRecognized
	}

	userTag, err := names.ParseUserTag(userTagString)
	if err != nil {
		zapctx.Error(ctx, "failed to parse caveat user tag", zap.Error(err))
		return nil, checkers.ErrCaveatNotRecognized
	}

	offerTag := jimmnames.NewApplicationOfferTag(offerUUID)

	user := openfga.NewUser(
		&dbmodel.Identity{
			Name: userTag.Id(),
		},
		md.ofgaClient,
	)

	allowed, err := openfga.CheckRelation(ctx, user, offerTag, ofganames.ConsumerRelation)
	if err != nil {
		zapctx.Error(ctx, "failed to check request caveat relation", zap.Error(err))
		return nil, errors.E(err)
	}

	if allowed {
		return []checkers.Caveat{
			checkers.DeclaredCaveat("offer-uuid", offerUUID),
			checkers.TimeBeforeCaveat(time.Now().Add(defaultDischargeExpiry)),
		}, nil
	}
	zapctx.Debug(ctx, "macaroon dishcharge denied", zap.String("user", user.Name), zap.String("offer", offerUUID))
	return nil, httpbakery.ErrPermissionDenied
}
