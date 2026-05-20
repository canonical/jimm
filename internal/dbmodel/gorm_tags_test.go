// Copyright 2026 Canonical.

package dbmodel_test

import (
	"sync"
	"testing"

	qt "github.com/frankban/quicktest"
	"gorm.io/gorm/schema"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func parseSchema(c *qt.C, model any) *schema.Schema {
	c.Helper()

	s, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	c.Assert(err, qt.IsNil)
	return s
}

func TestGormDefaultColumnMappings(t *testing.T) {
	c := qt.New(t)

	controllerSchema := parseSchema(c, &dbmodel.Controller{})
	c.Check(controllerSchema.LookUpField("TLSHostname").DBName, qt.Equals, "tls_hostname")

	controllerProfileSchema := parseSchema(c, &dbmodel.ControllerProfile{})
	c.Check(controllerProfileSchema.LookUpField("JujuVersion").DBName, qt.Equals, "juju_version")

	groupSchema := parseSchema(c, &dbmodel.GroupEntry{})
	c.Check(groupSchema.LookUpField("Name").DBName, qt.Equals, "name")
	c.Check(groupSchema.LookUpField("UUID").DBName, qt.Equals, "uuid")

	roleSchema := parseSchema(c, &dbmodel.RoleEntry{})
	c.Check(roleSchema.LookUpField("Name").DBName, qt.Equals, "name")
	c.Check(roleSchema.LookUpField("UUID").DBName, qt.Equals, "uuid")
}

func TestGormRetainedNonDefaultMappings(t *testing.T) {
	c := qt.New(t)

	modelSchema := parseSchema(c, &dbmodel.Model{})
	ownerRelation := modelSchema.Relationships.Relations["Owner"]
	c.Assert(ownerRelation, qt.IsNotNil)
	c.Assert(ownerRelation.References, qt.HasLen, 1)
	c.Check(ownerRelation.References[0].ForeignKey.DBName, qt.Equals, "owner_identity_name")
	c.Check(ownerRelation.References[0].PrimaryKey.DBName, qt.Equals, "name")

	sshKeySchema := parseSchema(c, &dbmodel.SSHKey{})
	modelRelation := sshKeySchema.Relationships.Relations["Model"]
	c.Assert(modelRelation, qt.IsNotNil)
	c.Assert(modelRelation.References, qt.HasLen, 1)
	c.Check(modelRelation.References[0].ForeignKey.DBName, qt.Equals, "model_uuid")
	c.Check(modelRelation.References[0].PrimaryKey.DBName, qt.Equals, "uuid")

	userMappingSchema := parseSchema(c, &dbmodel.UserMapping{})
	c.Check(userMappingSchema.LookUpField("ExternalUserName").DBName, qt.Equals, "external_user")

	jobLogSchema := parseSchema(c, &dbmodel.JobLog{})
	c.Check(jobLogSchema.PrimaryFields, qt.HasLen, 2)
	c.Check(jobLogSchema.LookUpField("JobID").PrimaryKey, qt.IsTrue)
	c.Check(jobLogSchema.LookUpField("LineNumber").PrimaryKey, qt.IsTrue)
}
