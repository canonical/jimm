// Copyright 2023 canonical.

// Package names holds functions used by other jimm components to
// create valid OpenFGA tags.
package names

import (
	"fmt"

	"github.com/canonical/jimm/internal/errors"
	jimmnames "github.com/canonical/jimm/pkg/names"
	cofga "github.com/canonical/ofga"

	"github.com/juju/juju/core/permission"
	"github.com/juju/names/v5"
)

// Relation Types
var (
	// MemberRelation represents a member relation between entities.
	MemberRelation cofga.Relation = "member"
	// AdministratorRelation represents an administrator relation between entities.
	AdministratorRelation cofga.Relation = "administrator"
	// ControllerRelation represents a controller relation between entities.
	ControllerRelation cofga.Relation = "controller"
	// ModelRelation represents a model relation between entities.
	ModelRelation cofga.Relation = "model"
	// ConsumerRelation represents a consumer relation between entities.
	ConsumerRelation cofga.Relation = "consumer"
	// ReaderRelation represents a reader relation between entities.
	ReaderRelation cofga.Relation = "reader"
	// WriterRelation represents a writer relation between entities.
	WriterRelation cofga.Relation = "writer"
	// CanAddModelRelation represents a can_addmodel relation between entities.
	CanAddModelRelation cofga.Relation = "can_addmodel"
	// AuditLogViewer represents an audit_log_viewer relation between entities.
	AuditLogViewerRelation cofga.Relation = "audit_log_viewer"
	// NoRelation is returned when there is no relation.
	NoRelation cofga.Relation = ""
)

// allRelations contains a slice of all valid relations.
// NB: Add any new relations from the above to this slice.
var allRelations = []cofga.Relation{MemberRelation, AdministratorRelation, ControllerRelation, ModelRelation, ConsumerRelation, ReaderRelation, WriterRelation, CanAddModelRelation, AuditLogViewerRelation, NoRelation}

// EveryoneUser is the username representing all users and is treated uniquely when used in OpenFGA tuples.
const EveryoneUser = "everyone@external"

// Tag represents an entity tag as used by JIMM in OpenFGA.
type Tag = cofga.Entity

// ResourceTagger represents an entity tag that implements
// a method returning entity's id and kind.
// TODO(ale8k): Rename this to remove the "er", "er" should only apply to interfaces with a single method.
type ResourceTagger interface {
	names.UserTag |
		jimmnames.GroupTag |
		names.ControllerTag |
		names.ModelTag |
		names.ApplicationOfferTag |
		names.CloudTag |
		jimmnames.ServiceAccountTag

	Id() string
	Kind() string
}

// ConvertTagWithRelation converts a resource tag to an OpenFGA tag
// and adds a relation to it.
func ConvertTagWithRelation[RT ResourceTagger](t RT, relation cofga.Relation) *Tag {
	tag := ConvertTag(t)
	tag.Relation = relation
	return tag
}

// ConvertTag converts a resource tag to an OpenFGA tag where the resource tag is limited to
// specific types of tags.
func ConvertTag[RT ResourceTagger](t RT) *Tag {
	id := t.Id()
	if t.Kind() == names.UserTagKind && id == EveryoneUser {
		// A user with ID "*" represents "everyone" in OpenFGA and allows checks like
		// `user:bob reader type:my-resource` to return true without a separate query
		// for the user:everyone user.
		id = "*"
	}
	tag := &Tag{
		ID:   id,
		Kind: cofga.Kind(t.Kind()),
	}
	return tag
}

// ConvertGenericTag converts any tag implementing the names.tag interface to an OpenFGA tag.
func ConvertGenericTag(t names.Tag) *Tag {
	tag := &Tag{
		ID:   t.Id(),
		Kind: cofga.Kind(t.Kind()),
	}
	return tag
}

// BlankKindTag returns a tag of the specified kind with a blank id.
// This function should only be used when removing relations to a specific
// resource (e.g. we want to remove all controller relations to a specific
// applicationoffer resource, so we specify user as BlankKindTag("controller"))
func BlankKindTag(kind string) (*Tag, error) {
	switch kind {
	case names.UserTagKind, jimmnames.GroupTagKind,
		names.ControllerTagKind, names.ModelTagKind,
		names.ApplicationOfferTagKind, names.CloudTagKind,
		jimmnames.ServiceAccountTagKind:
		return &Tag{
			Kind: cofga.Kind(kind),
		}, nil
	default:
		return nil, errors.E("unknown tag kind")
	}
}

// ConvertJujuRelation takes a juju relation string and converts it to
// one appropriate for use with OpenFGA.
func ConvertJujuRelation(relation string) (cofga.Relation, error) {
	const op = errors.Op("ConvertJujuRelation")
	switch relation {
	case string(permission.AdminAccess):
		return AdministratorRelation, nil
	case string(permission.ReadAccess):
		return ReaderRelation, nil
	case string(permission.WriteAccess):
		return WriterRelation, nil
	case string(permission.ConsumeAccess):
		return ConsumerRelation, nil
	case string(permission.AddModelAccess):
		return CanAddModelRelation, nil
	// Below are controller specific permissions that
	// are not represented in JIMM's OpenFGA model.
	case string(permission.LoginAccess):
		return NoRelation, errors.E(op, "login access unused")
	case string(permission.SuperuserAccess):
		return NoRelation, errors.E(op, "superuser access unused")
	default:
		return NoRelation, errors.E(op, "unknown relation")
	}
}

// ParseRelation parses the relation string
func ParseRelation(relationString string) (cofga.Relation, error) {
	const op = errors.Op("ParseRelation")
	switch relationString {
	case "":
		return cofga.Relation(""), nil
	case ControllerRelation.String():
		return ControllerRelation, nil
	case ModelRelation.String():
		return ModelRelation, nil
	case MemberRelation.String():
		return MemberRelation, nil
	case AdministratorRelation.String():
		return AdministratorRelation, nil
	case ConsumerRelation.String():
		return ConsumerRelation, nil
	case ReaderRelation.String():
		return ReaderRelation, nil
	case WriterRelation.String():
		return WriterRelation, nil
	case CanAddModelRelation.String():
		return CanAddModelRelation, nil
	case AuditLogViewerRelation.String():
		return AuditLogViewerRelation, nil
	default:
		return cofga.Relation(""), errors.E(op, fmt.Sprintf("unknown relation %s", relationString))

	}
}
