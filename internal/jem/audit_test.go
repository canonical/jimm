// Copyright 2018 Canonical Ltd.

package jem_test

import (
	"time"

	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemtest"
	"github.com/CanonicalLtd/jem/internal/mgosession"
	"github.com/CanonicalLtd/jem/params"
)

type auditSuite struct {
	jemtest.IsolatedMgoSuite
	database *jem.Database
}

var _ = gc.Suite(&auditSuite{})

func (s *auditSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	pool := mgosession.NewPool(context.TODO(), s.Session, 1)
	s.database = jem.NewDatabase(context.TODO(), pool, "jem")
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
	now := time.Now().Truncate(time.Millisecond)
	content := params.AuditModelCreated{
		ID:      "someid",
		UUID:    "someuuid",
		Owner:   "someowner",
		Creator: "somecreator",
		Cloud:   "somecloud",
		Region:  "someregion",
		AuditEntryCommon: params.AuditEntryCommon{
			Type_:    params.AuditLogType(params.AuditModelCreated{}),
			Created_: now,
		},
	}
	err := s.database.AppendAudit(testContext, content)
	c.Assert(err, gc.Equals, nil)
	entries, err := s.database.GetAuditEntries(testContext, time.Time{}, time.Time{}, "")
	c.Assert(entries, gc.DeepEquals, params.AuditLogEntries{{
		Content: content,
	}})
}

func (s *auditSuite) TestAddAuditModelDestroyed(c *gc.C) {
	now := time.Now().Truncate(time.Millisecond)
	content := params.AuditModelDestroyed{
		ID:   "someid",
		UUID: "someuuid",
		AuditEntryCommon: params.AuditEntryCommon{
			Type_:    params.AuditLogType(params.AuditModelDestroyed{}),
			Created_: now,
		},
	}
	err := s.database.AppendAudit(testContext, content)
	c.Assert(err, gc.Equals, nil)
	entries, err := s.database.GetAuditEntries(testContext, time.Time{}, time.Time{}, "")
	c.Assert(entries, gc.DeepEquals, params.AuditLogEntries{{
		Content: content,
	}})
}

func (s *auditSuite) TestGetAuditEntries(c *gc.C) {
	now := time.Now().Truncate(time.Millisecond)
	created := params.AuditModelCreated{
		ID:      "someid",
		UUID:    "someuuid",
		Owner:   "someowner",
		Creator: "somecreator",
		Cloud:   "somecloud",
		Region:  "someregion",
		AuditEntryCommon: params.AuditEntryCommon{
			Type_:    params.AuditLogType(params.AuditModelCreated{}),
			Created_: now,
		},
	}
	err := s.database.AppendAudit(testContext, created)
	c.Assert(err, gc.Equals, nil)
	content := params.AuditModelDestroyed{
		ID:   "someid",
		UUID: "someuuid",
		AuditEntryCommon: params.AuditEntryCommon{
			Type_:    params.AuditLogType(params.AuditModelDestroyed{}),
			Created_: now,
		},
	}
	err = s.database.AppendAudit(testContext, content)
	c.Assert(err, gc.Equals, nil)
	entries, err := s.database.GetAuditEntries(testContext, time.Time{}, time.Time{}, "")
	c.Assert(entries, gc.DeepEquals, params.AuditLogEntries{{
		Content: created,
	}, {
		Content: content,
	}})
}
