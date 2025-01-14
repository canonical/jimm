// Copyright 2025 Canonical.

package sshkeys

import (
	gossh "golang.org/x/crypto/ssh"
)

// PublicKey holds a public key and key comment.
// The public key is any key that is supported by
// the crypto/ssh PublicKey interface.
type PublicKey struct {
	gossh.PublicKey
	Comment string
}

func (pk PublicKey) valid() (ok bool, reason string) {
	if pk.PublicKey == nil {
		return false, "public key is nil"
	}
	if len(pk.Comment) > 255 {
		return false, "comment is too long (max 255 characters)"
	}
	return true, ""
}
