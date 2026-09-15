// Copyright 2026 Canonical.

package testing

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/api/client/modelmanager"
	jujurpc "github.com/juju/juju/rpc"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

// redirectingDialer wraps a juju.Dialer so that the first ModelInfo call
// on the given controller returns a redirect error pointing at the same
// controller. This simulates the redirect a Juju controller returns after
// an internal (JIMM-to-JIMM) model migration, without running a real
// migration. All dials still go to the real controller with real
// caller-scoped credentials.
type redirectingDialer struct {
	juju.Dialer
	ctlName    string
	redirected bool
}

func (d *redirectingDialer) DialControllerAsUser(ctx context.Context, u *openfga.User, ctl *dbmodel.Controller, resourceTags ...names.Tag) (juju.API, error) {
	api, err := d.Dialer.DialControllerAsUser(ctx, u, ctl, resourceTags...)
	if err != nil {
		return nil, err
	}
	if ctl.Name != d.ctlName || d.redirected {
		return api, nil
	}
	d.redirected = true
	return &redirectingAPI{API: api, ctlName: d.ctlName}, nil
}

// redirectingAPI wraps an API so its first ModelInfo call returns a
// redirect error as if the model had been migrated to another controller.
type redirectingAPI struct {
	juju.API
	ctlName string
}

func (a *redirectingAPI) ModelInfo(ctx context.Context, mt names.ModelTag) (jujuclient.ModelInfo, error) {
	return jujuclient.ModelInfo{}, &jujurpc.RequestError{
		Message: "model has been migrated to another controller",
		Code:    jujuparams.CodeRedirect,
		Info: jujuparams.RedirectErrorInfo{
			ControllerAlias: a.ctlName,
		}.AsMap(),
	}
}

// TestModelInfoAfterInternalMigrationRedirect verifies that when a
// controller returns a redirect error for ModelInfo (as happens after an
// internal model migration), JIMM re-dials the target controller as the
// calling user with their real permissions and serves the model info from
// there. The redirect is synthetic, but both dials hit the real Juju
// controller with caller-scoped JWTs, so the test proves Juju accepts the
// caller-scoped token on the re-dial.
func TestModelInfoAfterInternalMigrationRedirect(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	model := s.CreateModelForBob(c)
	ctrlName := model.Controller.Name

	// Put the model into internal-migration mode, as
	// initiateMigration would.
	model.MigrationMode = dbmodel.MigrationModeMigrateInternal
	err := s.JIMM.Database.UpdateModel(c.Context(), model)
	c.Assert(err, qt.IsNil)

	// Wrap the real dialer so the first ModelInfo on the source
	// controller returns a redirect to the same controller.
	s.JIMM.JujuManager.Dialer = &redirectingDialer{
		Dialer:  s.JIMM.JujuManager.Dialer,
		ctlName: ctrlName,
	}

	// Call ModelInfo as bob (non-admin model owner) through JIMM's API.
	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	var info jujuparams.ModelInfo
	var lastErr error
	// The re-dial serves the model from the target controller; since no
	// real migration ran, the model exists there (same controller), so
	// the call must succeed with real model data.
	for range 10 {
		results, err := client.ModelInfo([]names.ModelTag{names.NewModelTag(model.UUID.String)})
		if err == nil && len(results) == 1 && results[0].Error == nil {
			info = *results[0].Result
			lastErr = nil
			break
		}
		lastErr = err
		if results != nil && len(results) == 1 && results[0].Error != nil {
			lastErr = results[0].Error
		}
		time.Sleep(time.Second)
	}
	c.Assert(lastErr, qt.IsNil)
	c.Check(info.UUID, qt.Equals, model.UUID.String)
	c.Check(info.Name, qt.Equals, model.Name)

	// JIMM must have repointed the model at the target controller and
	// cleared the migration mode.
	var updated dbmodel.Model
	updated.SetTag(model.ResourceTag())
	err = s.JIMM.Database.GetModel(c.Context(), &updated)
	c.Assert(err, qt.IsNil)
	c.Check(updated.MigrationMode, qt.Equals, dbmodel.MigrationModeNone)
}
