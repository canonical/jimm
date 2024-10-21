// Copyright 2024 Canonical.

package cloudcred_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/jimm/cloudcred"
)

func TestIsVisibleAttribute(t *testing.T) {
	qt.Check(t, cloudcred.IsVisibleAttribute("ec2", "access-key", "access-key"), qt.Equals, true)
	qt.Check(t, cloudcred.IsVisibleAttribute("ec2", "access-key", "secret-key"), qt.Equals, false)
	qt.Check(t, cloudcred.IsVisibleAttribute("ec2", "unknown-auth", "access-key"), qt.Equals, false)
}
