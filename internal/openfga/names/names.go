// Copyright 2023 CanonicalLtd.

// Package names holds functions used by other jimm components to
// create valid OpenFGA tags.
package names

import (
	"github.com/CanonicalLtd/jimm/internal/errors"
	jimmnames "github.com/CanonicalLtd/jimm/pkg/names"
	cofga "github.com/canonical/ofga"
	"github.com/juju/juju/core/permission"
	"github.com/juju/names/v4"
)

// Kind represents the type of a tag kind.
type Kind = cofga.Kind

// Relation holds the type of tag relation.
type Relation = cofga.Relation

var (
	// MemberRelation represents a member relation between entities.
	MemberRelation Relation = "member"
	// AdministratorRelation represents an administrator relation between entities.
	AdministratorRelation Relation = "administrator"
	// ControllerRelation represents a controller relation between entities.
	ControllerRelation Relation = "controller"
	// ModelRelation represents a model relation between entities.
	ModelRelation Relation = "model"
	// ConsumerRelation represents a consumer relation between entities.
	ConsumerRelation Relation = "consumer"
	// ReaderRelation represents a reader relation between entities.
	ReaderRelation Relation = "reader"
	// WriterRelation represents a writer relation between entities.
	WriterRelation Relation = "writer"
	// CanAddModelRelation represents a can_addmodel relation between entities.
	CanAddModelRelation Relation = "can_addmodel"
	// AuditLogViewer represents an audit_log_viewer relation between entities.
	AuditLogViewerRelation Relation = "audit_log_viewer"
	// NoRelation is returned when there is no relation.
	NoRelation Relation = ""
)

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
		names.CloudTag

	Id() string
	Kind() string
}

// ConvertTagWithRelation converts a resource tag to an OpenFGA tag
// and adds a relation to it.
func ConvertTagWithRelation[RT ResourceTagger](t RT, relation Relation) *Tag {
	tag := ConvertTag(t)
	tag.Relation = relation
	return tag
}

// ConvertTag converts a resource tag to an OpenFGA tag where the resource tag is limited to
// specific types of tags.
func ConvertTag[RT ResourceTagger](t RT) *Tag {
	tag := &Tag{
		ID:   t.Id(),
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

// TBD
// TagFromString converts an entity tag to an OpenFGA tag.
// func TagFromString(t string) (*Tag, error) {
// 	entity, err := cofga.ParseEntity(t)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &entity, nil

// TBD
// tokens := strings.Split(t, ":")
// if len(tokens) != 2 {
// 	return nil, errors.E("unexpected tag format")
// }
// idTokens := strings.Split(tokens[1], "#")
// switch tokens[0] {
// case names.UserTagKind, jimmnames.GroupTagKind,
// 	names.ControllerTagKind, names.ModelTagKind,
// 	names.ApplicationOfferTagKind, names.CloudTagKind:
// 	switch len(idTokens) {
// 	case 1:
// 		return &Tag{
// 			kind: tokens[0],
// 			id:   tokens[1],
// 		}, nil
// 	case 2:
// 		return &Tag{
// 			kind:     tokens[0],
// 			id:       idTokens[0],
// 			relation: Relation(idTokens[1]),
// 		}, nil
// 	default:
// 		return nil, errors.E("invalid relation specifier")
// 	}
// default:
// 	return nil, errors.E("unknown tag kind")
// }
// }

// BlankKindTag returns a tag of the specified kind with a blank id.
// This function should only be used when removing relations to a specific
// resource (e.g. we want to remove all controller relations to a specific
// applicationoffer resource, so we specify user as BlankKindTag("controller"))
func BlankKindTag(kind string) (*Tag, error) {
	switch kind {
	case names.UserTagKind, jimmnames.GroupTagKind,
		names.ControllerTagKind, names.ModelTagKind,
		names.ApplicationOfferTagKind, names.CloudTagKind:
		return &Tag{
			Kind: cofga.Kind(kind),
		}, nil
	default:
		return nil, errors.E("unknown tag kind")
	}
}

// ConvertJujuRelation takes a juju relation string and converts it to
// one appropriate for use with OpenFGA.
func ConvertJujuRelation(relation string) (Relation, error) {
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
		return NoRelation, errors.E("login access unused")
	case string(permission.SuperuserAccess):
		return NoRelation, errors.E("superuser access unused")
	default:
		return NoRelation, errors.E("unknown relation")
	}
}

// ParseRelation parses the relation string
func ParseRelation(relationString string) (Relation, error) {
	switch relationString {
	case "":
		return Relation(""), nil
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
	default:
		return Relation(""), errors.E("unknown relation")

	}
}
