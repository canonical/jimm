// Copyright 2025 Canonical.

package ssh

import (
	gossh "golang.org/x/crypto/ssh"
)

// GetFingerprintsFromPrivateKey returns the fingerprints of the host key.
func GetFingerprintsFromPrivateKey(privateKey []byte) (map[string]string, error) {
	key, err := gossh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"SHA256": gossh.FingerprintSHA256(key.PublicKey()),
		"MD5":    gossh.FingerprintLegacyMD5(key.PublicKey()),
	}, nil
}

// GetFingerprintsFromPublicKey returns the public key of the host key.
func GetPublicKeyFromPrivateKey(privateKey []byte) (string, error) {
	key, err := gossh.ParsePrivateKey(privateKey)
	if err != nil {
		return "", err
	}
	return string(gossh.MarshalAuthorizedKey(key.PublicKey())), nil
}
