// Copyright 2025 Canonical.

package ssh

import (
	"sync/atomic"

	gliderssh "github.com/gliderlabs/ssh"
)

type ForwardMessage = forwardMessage

func ConnCallbackWithCount(maxConcurrentConnections int) (gliderssh.ConnCallback, func() int32) {
	n := new(atomic.Int32)
	return connCallback(maxConcurrentConnections, n), n.Load
}
