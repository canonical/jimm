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

// redirectingDialer wraps a juju.Dialer and fakes the redirect a controller
// returns after an internal model migration. Only the first ModelInfo on
// ctlName is intercepted; everything else hits the real controller.
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

// redirectingAPI returns a redirect error from ModelInfo, as if the
// model had been migrated away.
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

// TestModelInfoAfterInternalMigrationRedirect checks that JIMM handles a
// post-migration redirect: it repoints the model and re-dials as the
// calling user. The redirect itself is faked (see redirectingDialer) since
// we only have one controller. The re-dial goes to real Juju with the
// caller's own permissions.
func TestModelInfoAfterInternalMigrationRedirect(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	model := s.CreateModelForBob(c)
	ctrlName := model.Controller.Name

	// Mark the model as mid-migration, as initiateMigration would.
	model.MigrationMode = dbmodel.MigrationModeMigrateInternal
	err := s.JIMM.Database.UpdateModel(c.Context(), model)
	c.Assert(err, qt.IsNil)

	s.JIMM.JujuManager.Dialer = &redirectingDialer{
		Dialer:  s.JIMM.JujuManager.Dialer,
		ctlName: ctrlName,
	}

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	var info jujuparams.ModelInfo
	var lastErr error
	for range 10 {
		results, err := client.ModelInfo([]names.ModelTag{names.NewModelTag(model.UUID.String)})
		if err == nil && len(results) == 1 && results[0].Error == nil {
			info = *results[0].Result
			lastErr = nil
			break
		}
		lastErr = err
		if len(results) == 1 && results[0].Error != nil {
			lastErr = results[0].Error
		}
		time.Sleep(time.Second)
	}
	c.Assert(lastErr, qt.IsNil)
	c.Check(info.UUID, qt.Equals, model.UUID.String)
	c.Check(info.Name, qt.Equals, model.Name)

	// The model must be repointed and out of migration mode.
	var updated dbmodel.Model
	updated.SetTag(model.ResourceTag())
	err = s.JIMM.Database.GetModel(c.Context(), &updated)
	c.Assert(err, qt.IsNil)
	c.Check(updated.MigrationMode, qt.Equals, dbmodel.MigrationModeNone)
}
