// Copyright 2024 Canonical.

package jimmtest

import (
	"time"

	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/pubsub"
)

var now = (time.Time{}).UTC().Round(time.Millisecond)

type Option func(j *jimm.JIMM)

var (
	UnsetCredentialStore Option = func(j *jimm.JIMM) {
		j.CredentialStore = nil
	}
)

func NewJIMM(t Tester, additionalParameters *jimm.Parameters, options ...Option) *jimm.JIMM {

	auth := NewMockOAuthAuthenticator(t, nil)

	p := jimm.Parameters{
		UUID:               uuid.NewString(),
		Dialer:             &Dialer{},
		Pubsub:             &pubsub.Hub{},
		JWTService:         &jimmjwx.JWTService{},
		OAuthAuthenticator: &auth,
	}

	if additionalParameters != nil {
		if additionalParameters.UUID != "" {
			p.UUID = additionalParameters.UUID
		}
		if additionalParameters.Dialer != nil {
			p.Dialer = additionalParameters.Dialer
		}
		if additionalParameters.Database != nil {
			p.Database = additionalParameters.Database
		}
		if additionalParameters.CredentialStore != nil {
			p.CredentialStore = additionalParameters.CredentialStore
		}
		if additionalParameters.Pubsub != nil {
			p.Pubsub = additionalParameters.Pubsub
		}
		if len(additionalParameters.ReservedCloudNames) > 0 {
			p.ReservedCloudNames = append(p.ReservedCloudNames, additionalParameters.ReservedCloudNames...)
		}
		if additionalParameters.OpenFGAClient != nil {
			p.OpenFGAClient = additionalParameters.OpenFGAClient
		}
		if additionalParameters.JWTService != nil {
			p.JWTService = additionalParameters.JWTService
		}
		if additionalParameters.OAuthAuthenticator != nil {
			p.OAuthAuthenticator = additionalParameters.OAuthAuthenticator
		}
	}

	if p.Database == nil {
		p.Database = &db.Database{
			DB: PostgresDB(t, func() time.Time { return now }),
		}
	}
	if p.CredentialStore == nil {
		p.CredentialStore = p.Database
	}
	if p.OpenFGAClient == nil {
		ofgaClient, _, _, err := SetupTestOFGAClient(t.Name())
		if err != nil {
			t.Fatalf("setting up openfga client: %v", err)
		}
		p.OpenFGAClient = ofgaClient
	}

	j, err := jimm.New(p)
	if err != nil {
		t.Fatalf("instantiating jimm: %v", err)
	}

	for _, option := range options {
		option(j)
	}

	return j
}
