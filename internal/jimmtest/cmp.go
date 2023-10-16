// Copyright 2020 Canonical Ltd.

package jimmtest

import (
	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	gc "gopkg.in/check.v1"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/dbmodel"
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
	cmpopts.IgnoreTypes(dbmodel.ModelHardDelete{}),
	cmpopts.IgnoreFields(dbmodel.ApplicationOffer{}, "ID", "CreatedAt", "UpdatedAt"),
	cmpopts.IgnoreFields(dbmodel.Cloud{}, "ID", "CreatedAt", "UpdatedAt"),
	cmpopts.IgnoreFields(dbmodel.CloudCredential{}, "CloudName", "OwnerUsername"),
	cmpopts.IgnoreFields(dbmodel.CloudRegion{}, "CloudName"),
	cmpopts.IgnoreFields(dbmodel.CloudRegionControllerPriority{}, "CloudRegionID", "ControllerID"),
	cmpopts.IgnoreFields(dbmodel.Controller{}, "ID"),
	cmpopts.IgnoreFields(dbmodel.CloudCredential{}, "ID", "CreatedAt", "UpdatedAt", "CloudName", "OwnerUsername"),
	cmpopts.IgnoreFields(dbmodel.CloudRegion{}, "CloudName", "ID", "CreatedAt", "UpdatedAt"),
	cmpopts.IgnoreFields(dbmodel.ControllerConfig{}, "ID", "CreatedAt", "UpdatedAt"),
	cmpopts.IgnoreFields(dbmodel.CloudRegionControllerPriority{}, "CloudRegionID", "ControllerID", "ID", "CreatedAt", "UpdatedAt"),
	cmpopts.IgnoreFields(dbmodel.Controller{}, "ID", "CreatedAt", "UpdatedAt"),
	cmpopts.IgnoreFields(dbmodel.Model{}, "ID", "CreatedAt", "UpdatedAt", "OwnerUsername", "ControllerID", "CloudRegionID", "CloudCredentialID"),
	cmpopts.IgnoreFields(dbmodel.UserModelAccess{}, "ModelID", "Username"),
	cmpopts.IgnoreFields(dbmodel.User{}, "ID", "CreatedAt", "UpdatedAt"),
	cmpopts.IgnoreFields(dbmodel.UserModelAccess{}, "ModelID", "Username", "ID", "CreatedAt", "UpdatedAt"),
	cmpopts.IgnoreFields(dbmodel.UserModelDefaults{}, "ID", "CreatedAt", "UpdatedAt"),
	cmpopts.IgnoreFields(dbmodel.CloudDefaults{}, "ID", "CreatedAt", "UpdatedAt"),
	cmpopts.IgnoreFields(dbmodel.UserCloudAccess{}, "ID", "CreatedAt", "UpdatedAt"),
)

// CmpEquals uses cmp.Diff (see http://godoc.org/github.com/google/go-cmp/cmp#Diff)
// to compare two values, passing opts to the comparer to enable custom
// comparison.
func CmpEquals(opts ...cmp.Option) gc.Checker {
	return &cmpEqualsChecker{
		CheckerInfo: &gc.CheckerInfo{
			Name:   "CmpEquals",
			Params: []string{"obtained", "expected"},
		},
		check: func(params []interface{}, names []string) (result bool, error string) {
			if diff := cmp.Diff(params[0], params[1], opts...); diff != "" {
				return false, diff
			}
			return true, ""
		},
	}
}

type cmpEqualsChecker struct {
	*gc.CheckerInfo
	check func(params []interface{}, names []string) (result bool, error string)
}

func (c *cmpEqualsChecker) Check(params []interface{}, names []string) (result bool, error string) {
	return c.check(params, names)
}
