// Copyright 2025 Canonical.

package discharger_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/google/uuid"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/discharger"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

type dischargerSuite struct {
	discharger *discharger.MacaroonDischarger

	validOfferUUID string
}

var _ = gc.Suite(&dischargerSuite{})

func (s *dischargerSuite) Init(c *qt.C) {
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)
	jimmUUID := uuid.NewString()
	cfg := discharger.MacaroonDischargerConfig{
		MacaroonExpiryDuration: 1 * time.Hour,
		ControllerUUID:         jimmUUID,
		PrivateKey:             "ly/dzsI9Nt/4JxUILQeAX79qZ4mygDiuYGqc2ZEiDEc=",
		PublicKey:              "izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk=",
	}
	s.validOfferUUID = uuid.NewString()
	authorizer := &mocks.OfferAuthorizer{
		IsUserConsumerForOfferFunc: func(ctx context.Context, userTag names.UserTag, offerTag names.ApplicationOfferTag) (bool, error) {
			if userTag.IsLocal() {
				if userTag.Id() == "local-user" && offerTag.Id() == s.validOfferUUID {
					return true, nil
				}
				return false, nil
			} else if userTag.Id() == "external-user@external.com" && offerTag.Id() == s.validOfferUUID {
				return true, nil
			}
			return false, nil
		},
	}
	s.discharger, err = discharger.NewMacaroonDischarger(cfg, db, authorizer)
	c.Assert(err, qt.IsNil)

}

func (s *dischargerSuite) TestCheckThirdPartyCaveat(c *qt.C) {
	tests := []struct {
		name          string
		condition     string
		expectedError error
	}{
		{
			name:          "valid local user and offer",
			condition:     fmt.Sprintf("is-consumer user-local-user %s", s.validOfferUUID),
			expectedError: nil,
		},
		{
			name:          "valid external user and offer",
			condition:     fmt.Sprintf("is-consumer user-external-user@external.com %s", s.validOfferUUID),
			expectedError: nil,
		},
		{
			name:          "invalid user and offer",
			condition:     fmt.Sprintf("is-consumer user-invalid-user %s", s.validOfferUUID),
			expectedError: httpbakery.ErrPermissionDenied,
		},
		{
			name:          "invalid condition format",
			condition:     "is-consumer local-user",
			expectedError: checkers.ErrCaveatNotRecognized,
		},
		{
			name:          "unknown relation string",
			condition:     "unknown-relation user-local-user offer-1",
			expectedError: checkers.ErrCaveatNotRecognized,
		},
	}

	for _, test := range tests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()
			cavInfo := &bakery.ThirdPartyCaveatInfo{
				Condition: []byte(test.condition),
			}
			_, err := s.discharger.CheckThirdPartyCaveat(ctx, nil, cavInfo, nil)
			c.Assert(err, qt.Equals, test.expectedError)
		})
	}
}

func TestDischarger(t *testing.T) {
	qtsuite.Run(qt.New(t), &dischargerSuite{})
}
