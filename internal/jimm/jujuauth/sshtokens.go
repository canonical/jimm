// Copyright 2025 Canonical.

package jujuauth

import (
	"context"
	"encoding/base64"

	"github.com/juju/juju/core/permission"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/jimmjwx"
)

const (
	sshPublicKeyClaim = "ssh_public_key"
)

// SSHTokenGenerator provides the ability to generate JWT tokens that
// authenticate and authorize SSH connections to Juju controllers.
type SSHTokenGenerator struct {
	jwtService JWTService
}

// newSSHTokenGenerator returns a new SSHTokenGenerator.
func newSSHTokenGenerator(jwtService JWTService) SSHTokenGenerator {
	return SSHTokenGenerator{
		jwtService: jwtService,
	}
}

// SSHTokenArgs holds the arguments needed to generate a
// JWT token for SSH authentication with Juju controllers.
type SSHTokenArgs struct {
	User           string
	ControllerUUID string
	ModelTag       names.Tag
	PublicKey      []byte
}

// NewSSHToken generates a JWT token with the correct claims
// for SSH authentication with Juju controllers.
// It is expected that this function is called after
// user authentication and authorization has taken place.
func (s *SSHTokenGenerator) NewSSHToken(ctx context.Context, tokenArgs SSHTokenArgs) ([]byte, error) {
	token, err := s.jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: tokenArgs.ControllerUUID,
		User:       tokenArgs.User,
		Access: map[string]string{
			tokenArgs.ModelTag.String(): string(permission.AdminAccess),
		},
		ExtraClaims: map[string]any{
			sshPublicKeyClaim: base64.StdEncoding.EncodeToString(tokenArgs.PublicKey),
		},
	})
	if err != nil {
		return nil, err
	}

	return token, nil
}
