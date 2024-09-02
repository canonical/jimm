// Copyright 2024 Canonical.

package jimmtest

import (
	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	gc "gopkg.in/check.v1"
	"gorm.io/gorm"

	"github.com/canonical/jimm/v3/internal/dbmodel"
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
func UserTagCompare(a, b dbmodel.Identity) bool {
	return a.Tag() == b.Tag()
}

// DBObjectEquals is an equlity checker for dbmodel objects that ignores
// metadata fields in the database objects.
var DBObjectEquals = qt.CmpEquals(
	cmpopts.EquateEmpty(),
	cmpopts.IgnoreTypes(gorm.Model{}),
	cmpopts.IgnoreFields(dbmodel.Cloud{}, "ID", "CreatedAt", "UpdatedAt"),
	cmpopts.IgnoreFields(dbmodel.CloudCredential{}, "CloudName", "OwnerIdentityName"),
	cmpopts.IgnoreFields(dbmodel.CloudRegion{}, "CloudName"),
	cmpopts.IgnoreFields(dbmodel.CloudRegionControllerPriority{}, "CloudRegionID", "ControllerID"),
	cmpopts.IgnoreFields(dbmodel.Controller{}, "ID", "UpdatedAt", "CreatedAt"),
	cmpopts.IgnoreFields(dbmodel.Model{}, "ID", "CreatedAt", "UpdatedAt", "OwnerIdentityName", "ControllerID", "CloudRegionID", "CloudCredentialID"),
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
