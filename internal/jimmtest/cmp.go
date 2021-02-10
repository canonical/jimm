// Copyright 2020 Canonical Ltd.

package jimmtest

import (
	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
)

// CloudTagComparer is a function that can be used with gocmp.Compare
// which determines if two database cloud objects are equal based on
// comparing the Tag values. This is often sufficient where the objects
// are embedded in another database object.
func CloudTagCompare(a, b dbmodel.Cloud) bool {
	return a.Tag() == b.Tag()
}

// ControllerTagComparer is a function that can be used with gocmp.Compare
// which determines if two database controller objects are equal based on
// comparing the Tag values. This is often sufficient where the objects
// are embedded in another database object.
func ControllerTagCompare(a, b dbmodel.Controller) bool {
	return a.Tag() == b.Tag()
}

// UserTagComparer is a function that can be used with gocmp.Compare which
// determines if two database user objects are equal based on comparing
// the Tag values. This is often sufficient where the objects are embedded
// in another database object.
func UserTagCompare(a, b dbmodel.User) bool {
	return a.Tag() == b.Tag()
}

// DBObjectEquals is an equlity checker for dbmodel objects that ignores
// metadata fields in the database objects.
var DBObjectEquals = qt.CmpEquals(
	cmpopts.EquateEmpty(),
	cmpopts.IgnoreTypes(gorm.Model{}),
	cmpopts.IgnoreFields(dbmodel.Cloud{}, "ID", "CreatedAt", "UpdatedAt"),
	cmpopts.IgnoreFields(dbmodel.CloudCredential{}, "CloudName", "OwnerID"),
	cmpopts.IgnoreFields(dbmodel.CloudRegion{}, "CloudName"),
	cmpopts.IgnoreFields(dbmodel.Machine{}, "ID", "CreatedAt", "UpdatedAt", "ModelID"),
	cmpopts.IgnoreFields(dbmodel.Model{}, "ID", "CreatedAt", "UpdatedAt", "OwnerID", "ControllerID", "CloudRegionID", "CloudCredentialID"),
	cmpopts.IgnoreFields(dbmodel.UserModelAccess{}, "ModelID", "UserID"),
)
