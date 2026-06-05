// Copyright 2026 Canonical.

package river

import (
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/riverqueue/river/rivertype"

	"github.com/canonical/jimm/v3/internal/rivertypes"
)

type stubRetryPolicy struct {
	nextRetry time.Time
}

func (p stubRetryPolicy) NextRetry(*rivertype.JobRow) time.Time {
	return p.nextRetry
}

func TestUpgradeFlowRetryPolicy_NextRetry(t *testing.T) {
	c := qt.New(t)
	now := time.Date(2026, time.June, 4, 12, 0, 0, 0, time.UTC)
	fallbackRetry := now.Add(10 * time.Minute)
	policy := &upgradeFlowRetryPolicy{
		defaultPolicy: stubRetryPolicy{nextRetry: fallbackRetry},
		timeNow: func() time.Time {
			return now
		},
	}

	testCases := []struct {
		about   string
		jobKind string
		attempt int
		want    time.Time
	}{
		{
			about:   "supervisor first retry",
			jobKind: rivertypes.UpgradeToJobKind,
			attempt: 1,
			want:    now.Add(30 * time.Second),
		},
		{
			about:   "migration second retry",
			jobKind: migrationWorkerArgs{}.Kind(),
			attempt: 2,
			want:    now.Add(2 * time.Minute),
		},
		{
			about:   "upgrade first retry",
			jobKind: upgradeWorkerArgs{}.Kind(),
			attempt: 1,
			want:    now.Add(30 * time.Second),
		},
		{
			about:   "other jobs keep default policy",
			jobKind: "bootstrap-controller",
			attempt: 1,
			want:    fallbackRetry,
		},
	}

	for _, testCase := range testCases {
		c.Run(testCase.about, func(c *qt.C) {
			got := policy.NextRetry(&rivertype.JobRow{Kind: testCase.jobKind, Attempt: testCase.attempt})
			c.Assert(got, qt.DeepEquals, testCase.want)
		})
	}
}
