// Copyright 2025 Canonical.

// Package description provides version-agnostic wrappers around major versions
// of `github.com/juju/description` which hold Juju model descriptions, to
// facilitate migrations between different Juju versions.
package description

import (
	"fmt"

	descriptionv10 "github.com/juju/description/v10"
	descriptionv9 "github.com/juju/description/v9"
	"github.com/juju/juju/environs/config"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/errors"
)

const latestDescriptionVersion = 10

// CloudCredentialArgs holds the arguments needed to set a cloud credential.
type CloudCredentialArgs struct {
	Owner      names.UserTag
	Cloud      names.CloudTag
	Name       string
	AuthType   string
	Attributes map[string]string
}

// Model provides a version-agnostic interface for accessing
// model description data during migrations.
type Model interface {
	Owner() names.UserTag
	Users() []User
	CloudRegion() string
	Applications() []Application
	SetOwner(owner names.UserTag)
	ClearUsers()
	CloudCredential() CloudCredential
	SetCloudCredential(args CloudCredentialArgs)
	Config() map[string]any
	Serialize() ([]byte, error)
}

// User represents a user in the model description.
type User interface {
	Name() names.UserTag
	Access() string
}

// Application represents an application in the model description.
type Application interface {
	Name() string
	Offers() []ApplicationOffer
}

// ApplicationOffer represents an application offer in the model description.
type ApplicationOffer interface {
	OfferUUID() string
	OfferName() string
	ACL() map[string]string
}

// CloudCredential represents the current cloud credential for the model.
type CloudCredential interface {
	Owner() string
	Cloud() string
	Name() string
	AuthType() string
	Attributes() map[string]string
}

// Deserialize creates a new Model instance by deserializing
// the provided raw description data according to the target controller version.
func Deserialize(raw []byte, targetControllerVersion version.Number) (Model, error) {
	version, err := migrationDescriptionVersion(targetControllerVersion)
	if err != nil {
		return nil, err
	}

	switch version {
	case 9:
		desc, err := descriptionv9.Deserialize(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize v9 model description: %w", err)
		}
		return &migrationDescriptionV9{desc: desc}, nil
	case 10:
		desc, err := descriptionv10.Deserialize(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize v10 model description: %w", err)
		}
		return &migrationDescriptionV10{desc: desc}, nil
	default:
		return nil, errors.New("unsupported description version")
	}
}

// migrationDescriptionVersion returns the migration description package
// version to use for a given controller version.
func migrationDescriptionVersion(controllerVersion version.Number) (int, error) {
	v := controllerVersion

	switch {
	case v.Compare(version.MustParse("3.6.9")) < 0:
		return 0, fmt.Errorf("unsupported controller version %s, must be at least 3.6.9", controllerVersion)
	case v.Compare(version.MustParse("3.6.9")) >= 0 && v.Compare(version.MustParse("3.6.12")) <= 0:
		return 9, nil
	case v.Compare(version.MustParse("3.6.13")) == 0:
		return 10, nil
	default:
		return latestDescriptionVersion, nil
	}
}

// TryDetermineModelUUID attempts to extract the model UUID from the
// migration description data using the latest known description format.
func TryDetermineModelUUID(raw []byte) (string, error) {
	model, err := descriptionv10.Deserialize(raw)
	if err != nil {
		return "", fmt.Errorf("failed to deserialize model description: %w", err)
	}
	modelUUIDStr, ok := model.Config()[config.UUIDKey].(string)
	if !ok {
		return "", fmt.Errorf("model config must contain a string value for key %q", config.UUIDKey)
	}
	return modelUUIDStr, nil
}
