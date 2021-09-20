package jimm

import (
	"context"
	"strings"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

const (
	agentVersionKey = "agent-version"
)

// SetModelDefaults writes new default model setting values for the specified cloud/region.
func (j *JIMM) SetModelDefaults(ctx context.Context, user *dbmodel.User, cloudTag names.CloudTag, region string, configs map[string]interface{}) error {
	const op = errors.Op("jimm.SetModelDefaults")

	var keys strings.Builder
	var needComma bool
	for k := range configs {
		if needComma {
			keys.WriteByte(',')
		}
		keys.WriteString(k)
		needComma = true
	}
	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		Tag:     cloudTag.String(),
		UserTag: user.Tag().String(),
		Action:  "set-model-defaults",
		Params: dbmodel.StringMap{
			"keys":   keys.String(),
			"region": region,
		},
	}
	defer j.addAuditLogEntry(&ale)

	fail := func(err error) error {
		ale.Params["err"] = err.Error()
		return err
	}

	for k := range configs {
		if k == agentVersionKey {
			return fail(errors.E(op, errors.CodeBadRequest, `agent-version cannot have a default value`))
		}
	}

	cloud := dbmodel.Cloud{
		Name: cloudTag.Id(),
	}
	err := j.Database.GetCloud(ctx, &cloud)
	if err != nil {
		return fail(errors.E(op, err))
	}
	if region != "" {
		found := false
		for _, r := range cloud.Regions {
			if r.Name == region {
				found = true
			}
		}
		if !found {
			return fail(errors.E(op, errors.CodeNotFound, "region not found"))
		}
	}
	err = j.Database.SetCloudDefaults(ctx, &dbmodel.CloudDefaults{
		Username: user.Username,
		CloudID:  cloud.ID,
		Region:   region,
		Defaults: configs,
	})
	if err != nil {
		return fail(errors.E(op, err))
	}
	ale.Success = true
	return nil
}

// UnsetModelDefaults resets  default model setting values for the specified cloud/region.
func (j *JIMM) UnsetModelDefaults(ctx context.Context, user *dbmodel.User, cloudTag names.CloudTag, region string, keys []string) error {
	const op = errors.Op("jimm.UnsetModelDefaults")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		Tag:     cloudTag.String(),
		UserTag: user.Tag().String(),
		Action:  "unset-model-defaults",
		Params: dbmodel.StringMap{
			"keys":   strings.Join(keys, ","),
			"region": region,
		},
	}
	defer j.addAuditLogEntry(&ale)

	defaults := dbmodel.CloudDefaults{
		Username: user.Username,
		Cloud: dbmodel.Cloud{
			Name: cloudTag.Id(),
		},
		Region: region,
	}
	err := j.Database.UnsetCloudDefaults(ctx, &defaults, keys)
	if err != nil {
		ale.Params["err"] = err.Error()
		return errors.E(op, err)
	}
	ale.Success = true
	return nil
}

// ModelDefaultsForCloud returns the default config values for the specified cloud.
func (j *JIMM) ModelDefaultsForCloud(ctx context.Context, user *dbmodel.User, cloudTag names.CloudTag) (jujuparams.ModelDefaultsResult, error) {
	const op = errors.Op("jimm.ModelDefaultsForCloud")
	result := jujuparams.ModelDefaultsResult{
		Config: make(map[string]jujuparams.ModelDefaults),
	}
	defaults, err := j.Database.ModelDefaultsForCloud(ctx, user, cloudTag)
	if err != nil {
		result.Error = &jujuparams.Error{
			Message: err.Error(),
			Code:    string(errors.ErrorCode(err)),
		}
		return result, errors.E(op, err)
	}

	for _, cloudDefaults := range defaults {
		for k, v := range cloudDefaults.Defaults {
			d := result.Config[k]
			if cloudDefaults.Region == "" {
				d.Default = v
			} else {
				d.Regions = append(d.Regions, jujuparams.RegionDefaults{
					RegionName: cloudDefaults.Region,
					Value:      v,
				})
			}
			result.Config[k] = d
		}
	}
	if err != nil {
		return jujuparams.ModelDefaultsResult{}, errors.E(op, err)
	}
	return result, nil
}
