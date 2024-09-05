// Copyright 2024 Canonical.
package rpc

import (
	"context"

	"github.com/juju/juju/api/base"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// ProxyStreams starts a simple proxy for 2 websockets.
// After starting the proxy we listen for the first error
// returned and then close both connections before waiting
// on the second connection to ensure it is cleaned up.
func ProxyStreams(ctx context.Context, src, dst base.Stream) {
	errChan := make(chan error, 2)
	go func() { errChan <- proxy(src, dst) }()
	go func() { errChan <- proxy(dst, src) }()
	firstErr := <-errChan
	if firstErr != nil {
		zapctx.Error(ctx, "error from stream proxy", zap.Error(firstErr))
	}
	dst.Close()
	src.Close()
	secondErr := <-errChan
	if secondErr != nil {
		zapctx.Error(ctx, "error from stream proxy", zap.Error(secondErr))
	}
}

func proxy(src base.Stream, dst base.Stream) error {
	for {
		var data map[string]any
		err := src.ReadJSON(&data)
		if err != nil {
			if unexpectedReadError(err) {
				return err
			}
			return nil
		}
		err = dst.WriteJSON(data)
		if err != nil {
			return err
		}
	}
}
