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

	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/openfga"
)

var isFingerprintRegexp = regexp.MustCompile("^[0-9a-f]{2}(:[0-9a-f]{2}){15}$")

// keyManagerFacade is intended to be a temporary struct used to emulate logic
// that will eventually live in jujuapi. This struct contains all
// the api layer logic for SSH key management methods that are currently
// used by the rpcProxy.
type keyManagerFacade struct {
	SSHKeyManager
	user *openfga.User
}

func (s *keyManagerFacade) ListKeys(ctx context.Context, args jujuparams.ListSSHKeys) (jujuparams.StringsResults, error) {
	keys, err := s.ListUserPublicKeys(ctx, s.user)
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

func (s *keyManagerFacade) AddKeys(ctx context.Context, args jujuparams.ModifyUserSSHKeys) (jujuparams.ErrorResults, error) {
	var res []jujuparams.ErrorResult
	errF := func(err error, msg string) jujuparams.ErrorResult {
		return jujuparams.ErrorResult{Error: &jujuparams.Error{
			Message: fmt.Sprintf("%s: %s", msg, err.Error()),
		}}
	}

	for i, key := range args.Keys {
		out, comment, _, _, err := gossh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			res = append(res, errF(err, fmt.Sprintf("Failed to parse key (entry %d)", i)))
		}
		jimmKey := sshkeys.PublicKey{
			PublicKey: out,
			Comment:   comment,
		}
		if err := s.AddUserPublicKey(ctx, s.user, jimmKey); err != nil {
			res = append(res, errF(err, fmt.Sprintf("Failed to add key (comment %s)", comment)))
		}
	}

	return jujuparams.ErrorResults{Results: res}, nil
}

func (s *keyManagerFacade) DeleteKeys(ctx context.Context, args jujuparams.ModifyUserSSHKeys) (jujuparams.ErrorResults, error) {
	var res []jujuparams.ErrorResult
	errF := func(err error, msg string) jujuparams.ErrorResult {
		return jujuparams.ErrorResult{Error: &jujuparams.Error{
			Message: fmt.Sprintf("%s: %s", msg, err.Error()),
		}}
	}

	for _, key := range args.Keys {
		if isFingerprintRegexp.MatchString(key) {
			err := s.RemoveUserKeyByFingerprint(ctx, s.user, key)
			if err != nil {
				res = append(res, errF(err, fmt.Sprintf("Failed to remove key by fingerprint (%s)", key)))
			}
		} else {
			err := s.RemoveUserKeyByComment(ctx, s.user, key)
			if err != nil {
				res = append(res, errF(err, fmt.Sprintf("Failed to remove key by comment (%s)", key)))
			}
		}
	}

	return jujuparams.ErrorResults{Results: res}, nil
}

func marshalAuthorizedKeyWithComment(key sshkeys.PublicKey) string {
	// Copied from gossh.MarshalAuthorizedKey with an addition for the comment.
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

func fingerprintWithComment(key sshkeys.PublicKey) string {
	fingerprint := gossh.FingerprintLegacyMD5(key)
	return fmt.Sprintf("%s (%s)", fingerprint, key.Comment)
}
