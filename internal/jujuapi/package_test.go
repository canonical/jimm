// Copyright 2015 Canonical Ltd.

package jujuapi_test

import (
	"testing"

	jujutesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	jujutesting.MgoTestPackage(t)
}
