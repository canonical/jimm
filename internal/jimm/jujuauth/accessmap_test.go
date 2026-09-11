// Copyright 2026 Canonical.

package jujuauth

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// accessCheckerStub is a minimal GeneratorAccessChecker for the
// unexported helpers, recording calls and returning canned errors.
type accessCheckerStub struct {
	modelAccess map[string]string
	modelErr    error
	offerAccess map[string]string
	offerErr    error
	ctrlAccess  map[string]string
	ctrlErr     error
	cloudAccess map[string]string
	cloudErr    error
	permResult  map[string]string
	permErr     error
}

func (a *accessCheckerStub) GetUserModelAccess(_ context.Context, _ *openfga.User, mt names.ModelTag) (string, error) {
	if a.modelErr != nil {
		return "", a.modelErr
	}
	return a.modelAccess[mt.String()], nil
}

func (a *accessCheckerStub) GetUserApplicationOfferAccess(_ context.Context, _ *openfga.User, ot names.ApplicationOfferTag) (string, error) {
	if a.offerErr != nil {
		return "", a.offerErr
	}
	return a.offerAccess[ot.String()], nil
}

func (a *accessCheckerStub) GetUserControllerAccess(_ context.Context, _ *openfga.User, ct names.ControllerTag) (string, error) {
	if a.ctrlErr != nil {
		return "", a.ctrlErr
	}
	return a.ctrlAccess[ct.String()], nil
}

func (a *accessCheckerStub) GetUserCloudAccess(_ context.Context, _ *openfga.User, ct names.CloudTag) (string, error) {
	if a.cloudErr != nil {
		return "", a.cloudErr
	}
	return a.cloudAccess[ct.String()], nil
}

func (a *accessCheckerStub) CheckPermission(_ context.Context, _ *openfga.User, _ map[string]string, _ map[string]any) (map[string]string, error) {
	if a.permErr != nil {
		return nil, a.permErr
	}
	return a.permResult, nil
}

func TestResolveTargetAccessUnsupportedTag(t *testing.T) {
	c := qt.New(t)
	user := &openfga.User{Identity: &dbmodel.Identity{Name: "bob@external"}}
	_, err := resolveTargetAccess(context.Background(), user, names.NewUserTag("alice@external"), &accessCheckerStub{})
	c.Assert(err, qt.ErrorMatches, "unsupported resource tag type: .*")
}

func TestResolveTargetAccessModelError(t *testing.T) {
	c := qt.New(t)
	user := &openfga.User{Identity: &dbmodel.Identity{Name: "bob@external"}}
	mt := names.NewModelTag(uuid.New().String())
	_, err := resolveTargetAccess(context.Background(), user, mt, &accessCheckerStub{modelErr: errors.New("boom")})
	c.Assert(err, qt.ErrorMatches, "boom")
}

func TestResolveTargetAccessOfferError(t *testing.T) {
	c := qt.New(t)
	user := &openfga.User{Identity: &dbmodel.Identity{Name: "bob@external"}}
	ot := names.NewApplicationOfferTag(uuid.New().String())
	_, err := resolveTargetAccess(context.Background(), user, ot, &accessCheckerStub{offerErr: errors.New("boom")})
	c.Assert(err, qt.ErrorMatches, "boom")
}

func TestBuildAccessMapControllerAccessError(t *testing.T) {
	c := qt.New(t)
	user := &openfga.User{Identity: &dbmodel.Identity{Name: "bob@external"}}
	ct := names.NewControllerTag(uuid.New().String())
	_, err := buildAccessMap(context.Background(), user, nil, ct, dbmodel.Controller{}, &accessCheckerStub{ctrlErr: errors.New("nope")})
	c.Assert(err, qt.ErrorMatches, "nope")
}

func TestBuildAccessMapCloudAccessError(t *testing.T) {
	c := qt.New(t)
	user := &openfga.User{Identity: &dbmodel.Identity{Name: "bob@external"}}
	ct := names.NewControllerTag(uuid.New().String())
	ctl := dbmodel.Controller{
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			CloudRegion: dbmodel.CloudRegion{
				Cloud: dbmodel.Cloud{Name: "test-cloud"},
			},
		}},
	}
	_, err := buildAccessMap(context.Background(), user, nil, ct, ctl, &accessCheckerStub{
		ctrlAccess: map[string]string{ct.String(): "login"},
		cloudErr:   errors.New("cloud boom"),
	})
	c.Assert(err, qt.ErrorMatches, "failed to check user's cloud access: cloud boom")
}

func TestBuildAccessMapSkipsEmptyAccess(t *testing.T) {
	c := qt.New(t)
	user := &openfga.User{Identity: &dbmodel.Identity{Name: "bob@external"}}
	ct := names.NewControllerTag(uuid.New().String())
	mt := names.NewModelTag(uuid.New().String())
	accessMap, err := buildAccessMap(context.Background(), user, []names.Tag{mt}, ct, dbmodel.Controller{}, &accessCheckerStub{
		modelAccess: map[string]string{mt.String(): ""}, // empty -> skipped
		ctrlAccess:  map[string]string{ct.String(): "login"},
	})
	c.Assert(err, qt.IsNil)
	c.Assert(accessMap, qt.DeepEquals, map[string]string{ct.String(): "login"})
}
