// Copyright 2020 Canonical Ltd.package cloudcred

package cloudcred_test

import (
	"testing"

	"github.com/canonical/jimm/internal/cloudcred"
	qt "github.com/frankban/quicktest"
)

func TestIsVisibleAttribute(t *testing.T) {
	qt.Check(t, cloudcred.IsVisibleAttribute("ec2", "access-key", "access-key"), qt.Equals, true)
	qt.Check(t, cloudcred.IsVisibleAttribute("ec2", "access-key", "secret-key"), qt.Equals, false)
	qt.Check(t, cloudcred.IsVisibleAttribute("ec2", "unknown-auth", "access-key"), qt.Equals, false)
}
