// Copyright 2020 Canonical Ltd.package cloudcred

package cloudcred_test

import (
	"testing"

	"github.com/CanonicalLtd/jimm/internal/cloudcred"
	qt "github.com/frankban/quicktest"
)

func TestIsVisibleAttribute(t *testing.T) {
	qt.Check(t, cloudcred.IsVisibleAttribute("dummy", "userpass", "username"), qt.Equals, true)
	qt.Check(t, cloudcred.IsVisibleAttribute("dummy", "userpass", "password"), qt.Equals, false)
	qt.Check(t, cloudcred.IsVisibleAttribute("dummy", "unknown-auth", "username"), qt.Equals, false)
}
