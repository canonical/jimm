// Copyright 2026 Canonical.

package jujuclient

import (
	"context"
	"encoding/base64"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/openfga"
)

type fakeTokenMinter struct {
	called       bool
	resourceTags []names.Tag
	controller   *dbmodel.Controller
	user         *openfga.User
}

func (f *fakeTokenMinter) NewCallerLoginToken(ctx context.Context, resourceTags []names.Tag, ctl *dbmodel.Controller, user *openfga.User) ([]byte, error) {
	f.called = true
	f.resourceTags = resourceTags
	f.controller = ctl
	f.user = user
	return []byte("fake-jwt"), nil
}

func TestDialModelAsUserRejectsNilUser(t *testing.T) {
	c := qt.New(t)
	d := &Dialer{TokenMinter: &fakeTokenMinter{}}
	_, err := d.DialModelAsUser(context.Background(), nil, &dbmodel.Controller{}, names.ModelTag{})
	c.Assert(err, qt.ErrorMatches, "DialModelAsUser requires a non-nil user")
}

func TestDialControllerAsUserRejectsNilUser(t *testing.T) {
	c := qt.New(t)
	d := &Dialer{TokenMinter: &fakeTokenMinter{}}
	_, err := d.DialControllerAsUser(context.Background(), nil, &dbmodel.Controller{})
	c.Assert(err, qt.ErrorMatches, "DialControllerAsUser requires a non-nil user")
}

func TestCreateUserLoginRequestEncodesToken(t *testing.T) {
	c := qt.New(t)

	minter := &fakeTokenMinter{}
	d := &Dialer{TokenMinter: minter}

	user := &openfga.User{Identity: &dbmodel.Identity{Name: "bob@external"}}
	ctl := &dbmodel.Controller{UUID: uuid.New().String()}
	modelTag := names.NewModelTag(uuid.New().String())

	req, err := d.createUserLoginRequest(context.Background(), ctl, []names.Tag{modelTag}, user)
	c.Assert(err, qt.IsNil)

	c.Assert(req.AuthTag, qt.Equals, user.ResourceTag().String())
	c.Assert(req.Token, qt.Equals, base64.StdEncoding.EncodeToString([]byte("fake-jwt")))
	c.Assert(minter.called, qt.IsTrue)
	c.Assert(minter.user, qt.Equals, user)
	c.Assert(minter.resourceTags, qt.HasLen, 1)
	c.Assert(minter.resourceTags[0].String(), qt.Equals, modelTag.String())
}

func TestNewDialerWiresTokenMinter(t *testing.T) {
	c := qt.New(t)
	minter := &fakeTokenMinter{}
	jwtSvc := &jimmjwx.JWTService{}
	d := NewDialer(jwtSvc, minter, "test-uuid")
	c.Assert(d.TokenMinter, qt.Equals, minter)
	c.Assert(d.JWTService, qt.Equals, jwtSvc)
	c.Assert(d.AdminUsername, qt.Equals, "jaas-test-uuid@external")
}
