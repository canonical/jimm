// Copyright 2023 Canonical Ltd.

package cmd_test

import (
	"context"

	ofga "github.com/CanonicalLtd/jimm/internal/openfga"
	gc "gopkg.in/check.v1"
)

type fgaSuite struct {
	jimmSuite
}

func (s *fgaSuite) SetUpTest(c *gc.C) {
	s.jimmSuite.SetUpTest(c)

}

func (s *fgaSuite) TearDownTest(c *gc.C) {
	s.jimmSuite.TearDownTest(c)
	_ = ofga.TruncateOpenFgaTuples(context.Background())
}
