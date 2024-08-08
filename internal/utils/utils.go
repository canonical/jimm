// Copyright 2024 Canonical.
package utils

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// NewConversationID generates a unique ID that is used for the
// lifetime of a websocket connection.
func NewConversationID() string {
	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	if err != nil {
		zapctx.Error(context.Background(), "failed to generate rand", zap.Error(err))

	}
	return hex.EncodeToString(buf)
}
