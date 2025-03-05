// Copyright 2025 Canonical.

package rpcproxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"regexp"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/utils/v3/ssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/openfga"
)

var isFingerprintRegexp = regexp.MustCompile("^[0-9a-f]{2}(:[0-9a-f]{2}){15}$")

// keyManagerFacade is intended to be a temporary struct used to emulate logic
// that will eventually live in jujuapi. This struct contains all
// the api layer logic for SSH key management methods that are currently
// used by the rpcProxy.
type keyManagerFacade struct {
	keyManager SSHKeyManager
	user       *openfga.User
	modelUUID  string
}

// ListKeys lists the authenticated user's SSH keys
// in the format defined in args.
func (s *keyManagerFacade) ListKeys(ctx context.Context, args jujuparams.ListSSHKeys) (jujuparams.StringsResults, error) {
	keys, err := s.keyManager.ListUserPublicKeys(ctx, s.user, db.SSHKeyModelFilter{ModelUUID: s.modelUUID})
	if err != nil {
		return jujuparams.StringsResults{}, err
	}

	var formatter func(key sshkeys.PublicKey) string
	switch args.Mode {
	case ssh.FullKeys:
		formatter = marshalAuthorizedKeyWithComment
	case ssh.Fingerprints:
		formatter = fingerprintWithComment
	default:
		return jujuparams.StringsResults{}, fmt.Errorf("unknown mode (%v)", args.Mode)
	}

	// The Juju CLI takes the first element from the slice of stringsResults.
	res := jujuparams.StringsResult{}
	for _, key := range keys {
		res.Result = append(res.Result, formatter(key))
	}
	return jujuparams.StringsResults{Results: []jujuparams.StringsResult{res}}, nil
}

// AddKeys saves the SSH keys defined in args and associates them
// with the authenticated user.
func (s *keyManagerFacade) AddKeys(ctx context.Context, args jujuparams.ModifyUserSSHKeys) (jujuparams.ErrorResults, error) {
	var res []jujuparams.ErrorResult
	errF := func(err error, msg string) jujuparams.ErrorResult {
		return jujuparams.ErrorResult{Error: &jujuparams.Error{
			Code:    string(errors.ErrorCode(err)),
			Message: fmt.Sprintf("%s: %s", msg, err.Error()),
		}}
	}

	for i, key := range args.Keys {
		out, comment, _, _, err := gossh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			res = append(res, errF(err, fmt.Sprintf("Failed to parse key (entry %d)", i)))
		}
		parsedKey := sshkeys.PublicKey{
			PublicKey: out,
			Comment:   comment,
		}
		if err := s.keyManager.AddUserPublicKey(ctx, s.user, db.SSHKeyModelFilter{ModelUUID: s.modelUUID}, parsedKey); err != nil {
			res = append(res, errF(err, fmt.Sprintf("Failed to add key (comment %s)", comment)))
		}
	}

	return jujuparams.ErrorResults{Results: res}, nil
}

// DeleteKeys removes saved keys associated with the authenticated user
// and finds keys to remove either by comment or fingerprint.
func (s *keyManagerFacade) DeleteKeys(ctx context.Context, args jujuparams.ModifyUserSSHKeys) (jujuparams.ErrorResults, error) {
	var res []jujuparams.ErrorResult
	errF := func(err error, msg string) jujuparams.ErrorResult {
		return jujuparams.ErrorResult{Error: &jujuparams.Error{
			Code:    string(errors.ErrorCode(err)),
			Message: fmt.Sprintf("%s: %s", msg, err.Error()),
		}}
	}

	for _, key := range args.Keys {
		if isFingerprintRegexp.MatchString(key) {
			err := s.keyManager.RemoveUserKeyByFingerprint(ctx, s.user, db.SSHKeyModelFilter{ModelUUID: s.modelUUID}, key)
			if err != nil {
				res = append(res, errF(err, fmt.Sprintf("Failed to remove key by fingerprint (%s)", key)))
			}
		} else {
			err := s.keyManager.RemoveUserKeyByComment(ctx, s.user, db.SSHKeyModelFilter{ModelUUID: s.modelUUID}, key)
			if err != nil {
				res = append(res, errF(err, fmt.Sprintf("Failed to remove key by comment (%s)", key)))
			}
		}
	}

	return jujuparams.ErrorResults{Results: res}, nil
}

// marshalAuthorizedKeyWithComment marshals a public key + comment
// into an OpenSSH formatted authorized key string.
// Copied from gossh.MarshalAuthorizedKey with an addition for the comment.
func marshalAuthorizedKeyWithComment(key sshkeys.PublicKey) string {
	// Errors from the buffer's Write..() methods are always nil.
	b := &bytes.Buffer{}
	b.WriteString(key.Type())
	b.WriteByte(' ')
	e := base64.NewEncoder(base64.StdEncoding, b)
	_, _ = e.Write(key.Marshal())
	e.Close()
	b.WriteByte(' ')
	b.WriteString(key.Comment)
	b.WriteByte('\n')
	return b.String()
}

// fingerprintWithComment renders a short form version of a public
// key, displayed by the Juju CLI as '<fingerprint> (<comment>)'.
// Rendered on the server side as no strong types are defined for
// sharing keys between server and client.
func fingerprintWithComment(key sshkeys.PublicKey) string {
	fingerprint := gossh.FingerprintLegacyMD5(key)
	return fmt.Sprintf("%s (%s)", fingerprint, key.Comment)
}
