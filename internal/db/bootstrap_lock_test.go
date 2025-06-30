// Copyright 2025 Canonical.

package db_test

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	qt "github.com/frankban/quicktest"
)

func (s *dbSuite) TestBootstrap_LockAndUnlock(c *qt.C) {
	ctx := c.Context()
	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.Equals, nil)

	err = s.Database.LockBootstrap(context.Background(), 5*time.Minute)
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to acquire lock."))

	err = s.Database.LockBootstrap(ctx, 5*time.Minute)
	c.Assert(
		err,
		qt.ErrorMatches,
		"bootstrap lock is already held",
		qt.Commentf("Expected lock acquisition to fail in second session."),
	)

	err = s.Database.UnlockBootstrap(ctx)
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to release lock."))

	err = s.Database.LockBootstrap(ctx, 5*time.Minute)
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to acquire lock in second session."))
}

func (s *dbSuite) TestBootstrap_UnlockWithoutLock(c *qt.C) {
	ctx := c.Context()
	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.Equals, nil)

	err = s.Database.UnlockBootstrap(ctx)
	c.Assert(
		err,
		qt.ErrorMatches,
		"bootstrap lock is not held",
		qt.Commentf("Expected unlock to fail when no lock is held."),
	)
}

func (s *dbSuite) TestLockWithTTL(c *qt.C) {
	ctx := c.Context()
	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.Equals, nil)

	// Acquire the lock with a TTL of 0
	err = s.Database.LockBootstrap(ctx, 0)
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to acquire lock."))

	// Try to lock again, it should always succeed since the TTL is 0.
	err = s.Database.LockBootstrap(ctx, 5*time.Minute)
	c.Assert(err, qt.IsNil, qt.Commentf("Failed to acquire lock."))
}

func (s *dbSuite) TestConcurrentLock(c *qt.C) {
	ctx := c.Context()
	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.Equals, nil)

	lockAcquiredCounter := atomic.Int32{}
	lockNotAcquiredCounter := atomic.Int32{}
	wg := sync.WaitGroup{}
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.Database.LockBootstrap(context.Background(), 5*time.Minute)
			if err != nil {
				c.Check(
					err,
					qt.ErrorMatches,
					"bootstrap lock is already held",
					qt.Commentf("Expected lock acquisition to fail in second session."),
				)
				lockNotAcquiredCounter.Add(1)
				return
			}
			lockAcquiredCounter.Add(1)
		}()
	}
	wg.Wait()
	c.Assert(lockAcquiredCounter.Load(), qt.Equals, int32(1), qt.Commentf("Expected only one lock to be acquired concurrently."))
	c.Assert(lockNotAcquiredCounter.Load(), qt.Equals, int32(9), qt.Commentf("Expected 9 attempts to fail to acquire the lock concurrently."))
	err = s.Database.UnlockBootstrap(c.Context())
	c.Assert(err, qt.Equals, nil)
}
