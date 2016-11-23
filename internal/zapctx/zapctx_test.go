package zapctx_test

import (
	"bytes"
	"context"

	"github.com/juju/testing"
	"github.com/uber-go/zap"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/internal/zapctx"
)

type zapctxSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&zapctxSuite{})

func (*zapctxSuite) TestLogger(c *gc.C) {
	var buf bytes.Buffer
	logger := zap.New(zap.NewTextEncoder(), zap.Output(zap.AddSync(&buf)))
	ctx := zapctx.WithLogger(context.Background(), logger)
	zapctx.Logger(ctx).Info("hello")
	c.Assert(buf.String(), gc.Matches, `\[I\] .* hello\n`)
}

func (s *zapctxSuite) TestDefaultLogger(c *gc.C) {
	var buf bytes.Buffer
	logger := zap.New(zap.NewTextEncoder(), zap.Output(zap.AddSync(&buf)))

	s.PatchValue(&zapctx.Default, logger)
	zapctx.Logger(context.Background()).Info("hello")
	c.Assert(buf.String(), gc.Matches, `\[I\] .* hello\n`)
}
