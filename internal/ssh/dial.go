// Copyright 2025 Canonical.

package ssh

import (
	goerr "errors"
	"fmt"
	"net"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/errors"
	jimmssh "github.com/canonical/jimm/v3/internal/jimm/ssh"
)

// dialControllerSSHServer dials the controller ssh server, trying the addresses sequentially and returning a go ssh client.
func dialControllerSSHServer(connInfo jimmssh.ControllerInfo, destPort uint32) (*gossh.Client, error) {
	var client *gossh.Client
	var err error
	var errs []error
	for _, addr := range connInfo.Addresses {
		dest := net.JoinHostPort(addr, fmt.Sprint(destPort))
		client, err = gossh.Dial("tcp", dest, &gossh.ClientConfig{
			//nolint:gosec // this will be removed once we handle hostkeys
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Auth: []gossh.AuthMethod{
				gossh.PasswordCallback(func() (secret string, err error) {
					return connInfo.JWT, nil
				}),
			},
			Timeout: 5 * time.Second,
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	if client == nil {
		return nil, errors.E(goerr.Join(errs...), "cannot dial controller")
	}
	return client, nil
}
