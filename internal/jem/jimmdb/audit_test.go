// Copyright 2018 Canonical Ltd.

package jimmdb_test

import (
	"context"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"

	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/params"
)

type auditSuite struct {
	jemtest.IsolatedMgoSuite
	database *jimmdb.Database
}

var _ = gc.Suite(&auditSuite{})

func (s *auditSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	pool := mgosession.NewPool(context.TODO(), s.Session, 1)
	s.database = jimmdb.NewDatabase(context.TODO(), pool, "jem")
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
	pool.Close()
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *auditSuite) TearDownTest(c *gc.C) {
	s.database.Session.Close()
	s.database = nil
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *auditSuite) TestAddAuditModelCreated(c *gc.C) {
	content := params.AuditModelCreated{
		ID:      "someid",
		UUID:    "someuuid",
		Owner:   "someowner",
		Creator: "somecreator",
		Cloud:   "somecloud",
		Region:  "someregion",
	}
	s.database.AppendAudit(testContext, jemtest.NewIdentity("test"), &content)
	entries, err := s.database.GetAuditEntries(testContext, time.Time{}, time.Time{}, "")
	c.Assert(err, gc.Equals, nil)
	c.Assert(entries, jemtest.CmpEquals(cmpopts.IgnoreTypes(time.Time{})), params.AuditLogEntries{{
		&params.AuditModelCreated{
			ID:      "someid",
			UUID:    "someuuid",
			Owner:   "someowner",
			Creator: "somecreator",
			Cloud:   "somecloud",
			Region:  "someregion",
			AuditEntryCommon: params.AuditEntryCommon{
				Originator: "test",
				Type_:      params.AuditLogType(&params.AuditModelCreated{}),
			},
		},
	}})
}

func (s *auditSuite) TestAddAuditModelDestroyed(c *gc.C) {
	content := params.AuditModelDestroyed{
		ID:   "someid",
		UUID: "someuuid",
	}
	s.database.AppendAudit(testContext, jemtest.NewIdentity("test"), &content)
	entries, err := s.database.GetAuditEntries(testContext, time.Time{}, time.Time{}, "")
	c.Assert(err, gc.Equals, nil)
	c.Assert(entries, jemtest.CmpEquals(cmpopts.IgnoreTypes(time.Time{})), params.AuditLogEntries{{
		Content: &params.AuditModelDestroyed{
			ID:   "someid",
			UUID: "someuuid",
			AuditEntryCommon: params.AuditEntryCommon{
				Originator: "test",
				Type_:      params.AuditLogType(&params.AuditModelDestroyed{}),
			},
		},
	}})
}

func (s *auditSuite) TestAddAuditCloudCreated(c *gc.C) {
	content := params.AuditCloudCreated{
		ID:     "someid",
		Cloud:  "somecloud",
		Region: "someregion",
	}
	s.database.AppendAudit(testContext, jemtest.NewIdentity("test"), &content)
	entries, err := s.database.GetAuditEntries(testContext, time.Time{}, time.Time{}, "")
	c.Assert(err, gc.Equals, nil)
	c.Assert(entries, jemtest.CmpEquals(cmpopts.IgnoreTypes(time.Time{})), params.AuditLogEntries{{
		&params.AuditCloudCreated{
			ID:     "someid",
			Cloud:  "somecloud",
			Region: "someregion",
			AuditEntryCommon: params.AuditEntryCommon{
				Originator: "test",
				Type_:      params.AuditLogType(&params.AuditCloudCreated{}),
			},
		},
	}})
}

func (s *auditSuite) TestAddAuditCloudRemoved(c *gc.C) {
	content := params.AuditCloudRemoved{
		ID: "someid",
	}
	s.database.AppendAudit(testContext, jemtest.NewIdentity("test"), &content)
	entries, err := s.database.GetAuditEntries(testContext, time.Time{}, time.Time{}, "")
	c.Assert(err, gc.Equals, nil)
	c.Assert(entries, jemtest.CmpEquals(cmpopts.IgnoreTypes(time.Time{})), params.AuditLogEntries{{
		Content: &params.AuditCloudRemoved{
			ID: "someid",
			AuditEntryCommon: params.AuditEntryCommon{
				Originator: "test",
				Type_:      params.AuditLogType(&params.AuditCloudRemoved{}),
			},
		},
	}})
}

func (s *auditSuite) TestGetAuditEntries(c *gc.C) {
	created := params.AuditModelCreated{
		ID:      "someid",
		UUID:    "someuuid",
		Owner:   "someowner",
		Creator: "somecreator",
		Cloud:   "somecloud",
		Region:  "someregion",
	}
	s.database.AppendAudit(testContext, jemtest.NewIdentity("test"), &created)

	content := params.AuditModelDestroyed{
		ID:   "someid",
		UUID: "someuuid",
	}
	s.database.AppendAudit(testContext, jemtest.NewIdentity("test"), &content)
	entries, err := s.database.GetAuditEntries(testContext, time.Time{}, time.Time{}, "")
	c.Assert(err, gc.Equals, nil)
	c.Assert(entries, jemtest.CmpEquals(cmpopts.IgnoreTypes(time.Time{})), params.AuditLogEntries{{
		Content: &params.AuditModelCreated{
			ID:      "someid",
			UUID:    "someuuid",
			Owner:   "someowner",
			Creator: "somecreator",
			Cloud:   "somecloud",
			Region:  "someregion",
			AuditEntryCommon: params.AuditEntryCommon{
				Originator: "test",
				Type_:      params.AuditLogType(&params.AuditModelCreated{}),
			},
		},
	}, {
		Content: &params.AuditModelDestroyed{
			ID:   "someid",
			UUID: "someuuid",
			AuditEntryCommon: params.AuditEntryCommon{
				Originator: "test",
				Type_:      params.AuditLogType(&params.AuditModelDestroyed{}),
			},
		},
	}})
}
