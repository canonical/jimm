// Copyright 2026 Canonical.

package jujuapi

import (
	"context"

	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jujuapi/rpc"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

func init() {
	const facade = "JIMM_Metadata"
	facadeInit[facade] = func(r *controllerRoot) []int {
		r.AddMethod(facade, 1, "GetMetadata", rpc.Method(r.GetMetadata))
		r.AddMethod(facade, 1, "SetMetadata", rpc.Method(r.SetMetadata))
		r.AddMethod(facade, 1, "RemoveMetadata", rpc.Method(r.RemoveMetadata))
		r.AddMethod(facade, 1, "QueryMetadata", rpc.Method(r.QueryMetadata))
		return []int{1}
	}
}

// GetMetadata returns all metadata for the target model or application.
// The caller needs reader access to the target's model.
func (r *controllerRoot) GetMetadata(ctx context.Context, req params.GetMetadataRequest) (params.MetadataResults, error) {
	target, err := parseMetadataTarget(req.Entity)
	if err != nil {
		return params.MetadataResults{}, err
	}
	values, err := r.jimm.MetadataManager().GetMetadata(ctx, r.user, target)
	if err != nil {
		return params.MetadataResults{}, err
	}
	return params.MetadataResults{Metadata: values}, nil
}

// SetMetadata adds or updates the supplied keys and returns all metadata for
// the target. It leaves other keys unchanged and requires writer access.
func (r *controllerRoot) SetMetadata(ctx context.Context, req params.SetMetadataRequest) (params.MetadataResults, error) {
	target, err := parseMetadataTarget(req.Entity)
	if err != nil {
		return params.MetadataResults{}, err
	}
	values, err := r.jimm.MetadataManager().SetMetadata(ctx, r.user, target, req.Metadata)
	if err != nil {
		return params.MetadataResults{}, err
	}
	return params.MetadataResults{Metadata: values}, nil
}

// RemoveMetadata deletes the specified keys, or all keys if explicitly
// requested. The caller needs writer access to the target's model.
func (r *controllerRoot) RemoveMetadata(ctx context.Context, req params.RemoveMetadataRequest) error {
	target, err := parseMetadataTarget(req.Entity)
	if err != nil {
		return err
	}
	return r.jimm.MetadataManager().RemoveMetadata(ctx, r.user, target, req.All, req.Keys)
}

// QueryMetadata finds models and applications matching the exact key/value
// pair, restricted to models the caller can read.
func (r *controllerRoot) QueryMetadata(ctx context.Context, req params.QueryMetadataRequest) (params.QueryMetadataResponse, error) {
	if req.Matcher == nil || req.Matcher.Key == "" {
		return params.QueryMetadataResponse{}, errors.Codef(errors.CodeBadRequest, "a metadata filter with a key is required")
	}
	matches, err := r.jimm.MetadataManager().QueryMetadata(ctx, r.user, &db.EntityMetadataMatcher{Key: req.Matcher.Key, Value: req.Matcher.Value})
	if err != nil {
		return params.QueryMetadataResponse{}, err
	}
	results := make([]params.MetadataMatch, 0, len(matches))
	for _, row := range matches {
		target, err := metadataTargetToParams(row.EntityMetadataTarget)
		if err != nil {
			return params.QueryMetadataResponse{}, err
		}
		results = append(results, params.MetadataMatch{Entity: target, Key: row.Key, Value: row.Value})
	}
	return params.QueryMetadataResponse{Results: results}, nil
}

func parseMetadataTarget(target params.EntityMetadataTarget) (dbmodel.EntityMetadataTarget, error) {
	tag, err := names.ParseTag(target.EntityTag)
	if err != nil {
		return dbmodel.EntityMetadataTarget{}, errors.Codef(errors.CodeBadRequest, "%w", err)
	}
	switch tag := tag.(type) {
	case names.ModelTag:
		if target.ModelTag != "" && target.ModelTag != tag.String() {
			return dbmodel.EntityMetadataTarget{}, errors.Codef(errors.CodeBadRequest, "model tag must match the entity tag")
		}
		return dbmodel.EntityMetadataTarget{EntityType: tag.Kind(), ModelUUID: tag.Id(), Entity: tag.Id()}, nil
	case names.ApplicationTag:
		model, err := names.ParseModelTag(target.ModelTag)
		if err != nil {
			return dbmodel.EntityMetadataTarget{}, errors.Codef(errors.CodeBadRequest, "application metadata requires a valid model tag: %w", err)
		}
		return dbmodel.EntityMetadataTarget{EntityType: tag.Kind(), ModelUUID: model.Id(), Entity: tag.Id()}, nil
	default:
		return dbmodel.EntityMetadataTarget{}, errors.Codef(errors.CodeBadRequest, "unsupported metadata entity type %q", tag.Kind())
	}
}

func metadataTargetToParams(target dbmodel.EntityMetadataTarget) (params.EntityMetadataTarget, error) {
	switch target.EntityType {
	case names.ModelTagKind:
		tag := names.NewModelTag(target.ModelUUID)
		return params.EntityMetadataTarget{EntityTag: tag.String()}, nil
	case names.ApplicationTagKind:
		// The RPC only accepts valid application tags, but DB callers can write raw names.
		if !names.IsValidApplication(target.Entity) {
			return params.EntityMetadataTarget{}, errors.New("invalid application name in stored metadata")
		}
		return params.EntityMetadataTarget{
			EntityTag: names.NewApplicationTag(target.Entity).String(),
			ModelTag:  names.NewModelTag(target.ModelUUID).String(),
		}, nil
	default:
		return params.EntityMetadataTarget{}, errors.New("invalid entity type in stored metadata")
	}
}
