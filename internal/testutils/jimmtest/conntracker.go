// Copyright 2026 Canonical.

package jimmtest

import (
	"fmt"
	"strings"
	"time"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/rpc"
)

// CheckNoLeakedControllerConnections fails the test if any connection
// recorded by the rpc package registry is still open at cleanup. Register
// it before the JIMM service cleanup (cleanups run LIFO) so it runs after
// JIMM has shut down. Connections deregister themselves on Close, so
// anything left in the registry was never closed; the poll loop accounts
// for asynchronous teardown. The registry is cleared afterwards so
// leaked connections don't affect subsequent tests. Not safe for
// parallel tests: the registry is shared process-wide.
func CheckNoLeakedControllerConnections(c *qt.C, drainTimeout time.Duration) {
	c.Cleanup(func() {
		defer rpc.ResetConnTracking()

		deadline := time.Now().Add(drainTimeout)
		for {
			active := rpc.ActiveControllerConnections()
			if len(active) == 0 {
				return
			}
			if time.Now().After(deadline) {
				var sb strings.Builder
				for _, info := range active {
					fmt.Fprintf(&sb, "leaked connection to controller %q (model %q), dialed from:\n%s\n",
						info.Controller, info.ModelTag, info.DialStack)
				}
				c.Errorf("JIMM leaked %d controller connection(s):\n%s", len(active), sb.String())
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
}
