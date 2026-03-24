// Copyright 2025 Canonical.

package discharger

import (
	"context"
	"fmt"
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

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/errors"
)

var defaultDischargeExpiry = 15 * time.Minute

type MacaroonDischargerConfig struct {
	PublicKey              string
	PrivateKey             string
	MacaroonExpiryDuration time.Duration
	ControllerUUID         string
}

// OfferAuthorizer provides methods to check if a user is a consumer of an application offer.
type OfferAuthorizer interface {
	// IsUserConsumerForOffer checks if a user is a consumer of an application offer.
	IsUserConsumerForOffer(ctx context.Context, userTag names.UserTag, offerTag names.ApplicationOfferTag) (bool, error)
}

// NewMacaroonDischarger creates a new MacaroonDischarger instance with the provided configuration, database, and offer authorizer.
func NewMacaroonDischarger(cfg MacaroonDischargerConfig, db *db.Database, offerAuthorizer OfferAuthorizer) (*MacaroonDischarger, error) {
	var kp bakery.KeyPair
	if cfg.PublicKey == "" || cfg.PrivateKey == "" {
		return nil, errors.New("missing bakery private/public key")
	} else {
		if err := kp.Private.UnmarshalText([]byte(cfg.PrivateKey)); err != nil {
			return nil, fmt.Errorf("cannot unmarshal private key: %w", err)
		}
		if err := kp.Public.UnmarshalText([]byte(cfg.PublicKey)); err != nil {
			return nil, fmt.Errorf("cannot unmarshal public key: %w", err)
		}
	}
	if offerAuthorizer == nil {
		return nil, errors.New("userMappingManager cannot be nil")
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
		offerAuthorizer: offerAuthorizer,
		bakery:          b,
		kp:              kp,
	}, nil
}

type MacaroonDischarger struct {
	bakery          *bakery.Bakery
	kp              bakery.KeyPair
	offerAuthorizer OfferAuthorizer
}

// GetDischargerMux returns a mux that can handle macaroon bakery requests for the provided discharger.
func GetDischargerMux(macaroonDischarger *MacaroonDischarger, rootPath string) *http.ServeMux {
	discharger := httpbakery.NewDischarger(
		httpbakery.DischargerParams{
			Key:     &macaroonDischarger.kp,
			Checker: httpbakery.ThirdPartyCaveatCheckerFunc(macaroonDischarger.CheckThirdPartyCaveat),
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
func (md *MacaroonDischarger) CheckThirdPartyCaveat(ctx context.Context, _ *http.Request, cavInfo *bakery.ThirdPartyCaveatInfo, _ *httpbakery.DischargeToken) ([]checkers.Caveat, error) {
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
	offerTag := names.NewApplicationOfferTag(offerUUID)

	allowed, err := md.offerAuthorizer.IsUserConsumerForOffer(ctx, userTag, offerTag)
	if err != nil {
		zapctx.Error(ctx, "failed to check if user is consumer for offer", zap.Error(err), zap.String("user", userTagString), zap.String("offer", offerUUID))
		return nil, checkers.ErrCaveatNotRecognized
	}
	if allowed {
		return []checkers.Caveat{
			checkers.DeclaredCaveat("offer-uuid", offerUUID),
			checkers.TimeBeforeCaveat(time.Now().Add(defaultDischargeExpiry)),
		}, nil
	}
	zapctx.Debug(ctx, "macaroon dishcharge denied", zap.String("user", userTagString), zap.String("offer", offerUUID))
	return nil, httpbakery.ErrPermissionDenied
}
