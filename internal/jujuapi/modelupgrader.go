// Copyright 2026 Canonical.

package jujuapi

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v6"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jujuapi/rpc"
)

func init() {
	facadeInit["ModelUpgrader"] = func(r *controllerRoot) []int {
		abortModelUpgradeMethod := rpc.Method(r.AbortModelUpgrade)
		upgradeModelMethod := rpc.Method(r.UpgradeModel)

		// ModelUpgrader facade version 1 (Juju 3.6 clients).
		r.AddMethod("ModelUpgrader", 1, "AbortModelUpgrade", abortModelUpgradeMethod)
		r.AddMethod("ModelUpgrader", 1, "UpgradeModel", upgradeModelMethod)

		// ModelUpgrader facade version 2 (Juju 4.x clients). The methods are
		// wire-identical to version 1 (UpgradeModelParams, ModelParam and
		// UpgradeModelResult are unchanged between Juju 3.6 and 4.x), so the
		// same handlers are re-registered here.
		r.AddMethod("ModelUpgrader", 2, "AbortModelUpgrade", abortModelUpgradeMethod)
		r.AddMethod("ModelUpgrader", 2, "UpgradeModel", upgradeModelMethod)

		return []int{1, 2}
	}
}

// AbortModelUpgrade aborts and archives any in-progress upgrade for the given model.
// Aborting a model upgrade is idempotent, so if there is no in-progress upgrade, this method will still succeed.
func (r *controllerRoot) AbortModelUpgrade(ctx context.Context, args jujuparams.ModelParam) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	mt, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return errors.Codef(errors.CodeBadRequest, "%w", err)
	}
	return r.jimm.JujuManager().AbortModelUpgrade(ctx, r.user, mt)
}

// UpgradeModel upgrades the given model's agent to the specified version.
// If no target version is given (version.Zero), the controller selects the best version.
// If the target version is greater than the controller's agent version, the upgrade will fail.
func (r *controllerRoot) UpgradeModel(ctx context.Context, args jujuparams.UpgradeModelParams) (jujuparams.UpgradeModelResult, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	mt, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return jujuparams.UpgradeModelResult{}, errors.Codef(errors.CodeBadRequest, "%w", err)
	}

	chosenVersion, err := r.jimm.JujuManager().UpgradeModel(ctx, r.user, mt, args.TargetVersion, args.AgentStream, args.IgnoreAgentVersions, args.DryRun)
	if err != nil {
		return jujuparams.UpgradeModelResult{}, err
	}
	return jujuparams.UpgradeModelResult{ChosenVersion: chosenVersion}, nil
}
