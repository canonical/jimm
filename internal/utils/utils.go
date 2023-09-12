package utils

import (
	"crypto/rand"
	"encoding/hex"
)

// NewConversationID generates a unique ID that is used for the
// lifetime of a websocket connection.
func NewConversationID() string {
	buf := make([]byte, 8)
	rand.Read(buf) // Can't fail
	return hex.EncodeToString(buf)
}
