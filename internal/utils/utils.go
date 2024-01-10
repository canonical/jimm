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

// ToStringPtr returns a pointer to given string value. This should only be used
// as a helper function to get pointers to literals or function call return values.
func ToStringPtr(value string) *string {
	return &value
}
