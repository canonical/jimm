// Copyright 2026 Canonical.

package rpcproxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"

	jujuparams "github.com/juju/juju/rpc/params"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// keyManagerFacade implements the model-scoped KeyManager facade using JIMM's
// SSH key store.
type keyManagerFacade struct {
	keyManager SSHKeyManager
	user       *openfga.User
	modelUUID  string
}

// ListKeys lists the authenticated user's SSH keys in the requested format.
func (s *keyManagerFacade) ListKeys(ctx context.Context, args jujuparams.ListSSHKeys) (jujuparams.StringsResults, error) {
	keys, err := s.keyManager.ListUserPublicKeys(ctx, s.user, db.SSHKeyModelFilter{ModelUUID: s.modelUUID})
	if err != nil {
		return jujuparams.StringsResults{}, err
	}

	var formatter func(sshkeys.PublicKey) string
	switch args.Mode {
	case jujuparams.SSHListModeFull:
		formatter = marshalAuthorizedKeyWithComment
	case jujuparams.SSHListModeFingerprint:
		formatter = fingerprintWithComment
	default:
		return jujuparams.StringsResults{}, fmt.Errorf("unknown mode (%v)", args.Mode)
	}

	result := jujuparams.StringsResult{}
	for _, key := range keys {
		result.Result = append(result.Result, formatter(key))
	}
	return jujuparams.StringsResults{Results: []jujuparams.StringsResult{result}}, nil
}

// AddKeys saves each key and associates it with the authenticated user and model.
func (s *keyManagerFacade) AddKeys(ctx context.Context, args jujuparams.ModifyUserSSHKeys) (jujuparams.ErrorResults, error) {
	var results []jujuparams.ErrorResult
	errorResult := func(err error, message string) jujuparams.ErrorResult {
		return jujuparams.ErrorResult{Error: &jujuparams.Error{
			Code:    string(errors.ErrorCode(err)),
			Message: fmt.Sprintf("%s: %s", message, err),
		}}
	}

	for i, key := range args.Keys {
		publicKey, comment, _, _, err := gossh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			results = append(results, errorResult(err, fmt.Sprintf("failed to parse key (entry %d)", i)))
			continue
		}
		if err := s.keyManager.AddUserPublicKey(ctx, s.user, db.SSHKeyModelFilter{ModelUUID: s.modelUUID}, sshkeys.PublicKey{
			PublicKey: publicKey,
			Comment:   comment,
		}); err != nil {
			results = append(results, errorResult(err, fmt.Sprintf("failed to add key (comment %s)", comment)))
		}
	}

	return jujuparams.ErrorResults{Results: results}, nil
}

// DeleteKeys removes keys by fingerprint, comment, or full key value.
func (s *keyManagerFacade) DeleteKeys(ctx context.Context, args jujuparams.ModifyUserSSHKeys) (jujuparams.ErrorResults, error) {
	if err := s.keyManager.RemoveUserKeys(ctx, s.user, db.SSHKeyModelFilter{ModelUUID: s.modelUUID}, args.Keys...); err != nil {
		return jujuparams.ErrorResults{}, err
	}
	return jujuparams.ErrorResults{}, nil
}

// marshalAuthorizedKeyWithComment marshals an OpenSSH key including its comment.
func marshalAuthorizedKeyWithComment(key sshkeys.PublicKey) string {
	var buffer bytes.Buffer
	buffer.WriteString(key.Type())
	buffer.WriteByte(' ')
	encoder := base64.NewEncoder(base64.StdEncoding, &buffer)
	_, _ = encoder.Write(key.Marshal())
	_ = encoder.Close()
	if key.Comment != "" {
		buffer.WriteByte(' ')
		buffer.WriteString(key.Comment)
	}
	return buffer.String()
}

// fingerprintWithComment renders the short form used by the Juju CLI.
func fingerprintWithComment(key sshkeys.PublicKey) string {
	return fmt.Sprintf("%s (%s)", gossh.FingerprintLegacyMD5(key), key.Comment)
}
